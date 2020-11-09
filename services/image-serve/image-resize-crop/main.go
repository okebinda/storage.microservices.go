package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/disintegration/imaging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Response is the response sent to AWS API Gateway
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// validImageFormats defines valid image mime types for processing
var validImageFormats []string = []string{
	"image/png",
	"image/jpeg",
}

var logger *zap.SugaredLogger

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (Response, error) {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// get environment parameters
	sourceBucket := os.Getenv("AWS_S3_BUCKET_SOURCE")
	destinationBucket := os.Getenv("AWS_S3_BUCKET_DESTINATION")
	region := os.Getenv("REGION")
	maxWidth, err := strconv.Atoi(os.Getenv("MAX_WIDTH"))
	if err != nil {
		logger.Errorf("Could not convert MAX_WIDTH to int: %v", err)
		return serverErrorResponse(err)
	}
	maxHeight, err := strconv.Atoi(os.Getenv("MAX_HEIGHT"))
	if err != nil {
		logger.Errorf("Could not convert MAX_HEIGHT to int: %v", err)
		return serverErrorResponse(err)
	}

	// get path parameters
	size := request.PathParameters["size"]
	imageKey := request.PathParameters["image_key"]

	logger.Infow("Request parameters",
		"size", size,
		"imageKey", imageKey,
	)

	// simple sanity check
	if size == "" || imageKey == "" {
		logger.Errorf("Missing parameters, cannot complete request; size: %s, image_key: %s", size, imageKey)
		return userErrorResponse(fmt.Sprintf("Missing parameters, cannot complete request; size: %s, image_key: %s", size, imageKey))
	}

	// check size parameter is correct format
	isMatch, err := regexp.MatchString(`^\d+x\d+$`, size)
	if err != nil {
		logger.Errorf("Could not read parameter format, cannot complete request; size: %s: %v", size, err)
		return userErrorResponse(fmt.Sprintf("Could not read parameter format, cannot complete request; size: %s: %v", size, err))
	}
	if isMatch == false {
		logger.Errorf("Bad parameter format, cannot complete request; size: %s", size)
		return userErrorResponse(fmt.Sprintf("Bad parameter format, cannot complete request; size: %s", size))
	}

	// parse image dimensions from path
	sizes := strings.Split(size, "x")
	width, err := strconv.Atoi(sizes[0])
	if err != nil {
		logger.Errorf("Could not convert sizes[0] to int: %v", err)
		return userErrorResponse("Could not convert width to int.")
	}
	height, err := strconv.Atoi(sizes[1])
	if err != nil {
		logger.Errorf("Could not convert sizes[1] to int: %v", err)
		return userErrorResponse("Could not convert height to int.")
	}

	// initialize AWS session
	sess := session.Must(session.NewSession())

	// assign file names
	resizedFileKey := fmt.Sprintf("crop/%s/%s", size, imageKey)
	localFile := fmt.Sprintf("/tmp/%s", filepath.Base(imageKey))

	// create local temp file
	file, err := os.Create(localFile)
	if err != nil {
		logger.Errorf("os.Create() error: %s", err)
		return serverErrorResponse(err)
	}

	// download file from S3
	_, err = downloadFile(sess, file, sourceBucket, imageKey)
	if err != nil {
		logger.Errorf("S3 downloader error: %s, %s", imageKey, err)
		close(file)
		return serverErrorResponse(err)
	}

	// detect file type
	fileType, err := getFileType(file)
	if err != nil {
		logger.Errorf("File read error: %s", err)
		close(file)
		return serverErrorResponse(err)
	}

	// reject bad file types
	if !contains(validImageFormats, fileType) {
		logger.Errorf("Unsupported file type: %s", fileType)
		close(file)
		return userErrorResponse(fmt.Sprintf("Unsupported file type: %s", fileType))
	}

	// open image
	img, err := imaging.Open(localFile)
	if err != nil {
		logger.Errorf("Failed to open image: %v", err)
		close(file)
		return serverErrorResponse(err)
	}

	// resize image
	width = min(maxWidth, width)
	height = min(maxHeight, height)
	err = resizeImage(img, localFile, width, height)
	if err != nil {
		logger.Errorf("Failed to resize image: %v", err)
		close(file)
		return serverErrorResponse(err)
	}

	// upload to public bucket
	err = uploadFile(sess, file, destinationBucket, resizedFileKey, fileType)
	if err != nil {
		logger.Errorf("Failed to upload file: %s, %v", resizedFileKey, err)
		close(file)
		return serverErrorResponse(err)
	}

	logger.Infow("Image resize complete.",
		"bucket", destinationBucket,
		"file_key", resizedFileKey,
		"width", width,
		"height", height,
	)

	close(file)

	// response
	redirectURL := fmt.Sprintf("http://%s.s3-website-%s.amazonaws.com/%s", destinationBucket, region, resizedFileKey)
	return redirectResponse(redirectURL), nil
}

// sugaredLogger initializes the zap sugar logger
func sugaredLogger(requestID string) *zap.SugaredLogger {
	// zapLogger, err := zap.NewDevelopment()
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	return zapLogger.
		With(zap.Field{Key: "request_id", Type: zapcore.StringType, String: requestID}).
		Sugar()
}

// close closes a file and logs any errors
func close(file *os.File) {
	if err := file.Close(); err != nil {
		logger.Errorf("Error closing the file: %s", err)
	}
}

// downloadFile downloads a file from an S3 bucket
func downloadFile(sess *session.Session, file *os.File, bucketName, fileKey string) (int64, error) {
	downloader := s3manager.NewDownloader(sess)
	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(fileKey),
		})
	return numBytes, err
}

// getFileType detects the mime type of the given file
func getFileType(file *os.File) (string, error) {
	buff := make([]byte, 512)
	if _, err := file.Read(buff); err != nil {
		return "", err
	}
	fileType := http.DetectContentType(buff)
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	return fileType, nil
}

// contains tests if a slice contains a string
func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

// min returns the lesser of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// resizeImage resizes an image, cropping to widthxheight
func resizeImage(img image.Image, localFile string, widthIn, heightIn int) error {
	var err error
	img = imaging.Fill(img, widthIn, heightIn, imaging.Center, imaging.Lanczos)
	err = imaging.Save(img, localFile)
	return err
}

// uploadFile uploads a file to an S3 bucket
func uploadFile(sess *session.Session, file *os.File, bucketName, fileKey, fileType string) error {

	// Get file size and read the file content into a buffer
	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	if _, err := file.Read(buffer); err != nil {
		return err
	}

	// upload to public bucket
	_, err := s3.New(sess).PutObject(&s3.PutObjectInput{
		Bucket:             aws.String(bucketName),
		Key:                aws.String(fileKey),
		ACL:                aws.String("public-read"),
		Body:               bytes.NewReader(buffer),
		ContentLength:      aws.Int64(size),
		ContentType:        aws.String(fileType),
		ContentDisposition: aws.String("attachment"),
	})
	return err
}

// successResponse generates a redirect (301) response
func redirectResponse(redirectURL string) Response {
	return Response{
		StatusCode:      301,
		IsBase64Encoded: false,
		Body:            "",
		Headers: map[string]string{
			"location": redirectURL,
		},
	}
}

// userErrorResponse generates a user error (400) response
func userErrorResponse(errorMessage string) (Response, error) {
	body, err := json.Marshal(map[string]interface{}{
		"error": errorMessage,
	})
	if err != nil {
		logger.Errorf("Marshalling error: %s", err)
		return Response{StatusCode: 500}, err
	}
	return generateResponse(400, body), nil
}

// serverErrorResponse generates a server error (500) response
func serverErrorResponse(errorMessage error) (Response, error) {
	body, err := json.Marshal(map[string]interface{}{
		"error": "Server error",
	})
	if err != nil {
		logger.Errorf("Marshalling error: %s", err)
		return Response{StatusCode: 500}, err
	}
	return generateResponse(500, body), errorMessage
}

// generateResponse generates an HTTP JSON Lambda response to return to the user
func generateResponse(statusCode int, body []byte) Response {
	var buf bytes.Buffer
	json.HTMLEscape(&buf, body)
	return Response{
		StatusCode:      statusCode,
		IsBase64Encoded: false,
		Body:            buf.String(),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
}

func main() {
	lambda.Start(Handler)
}
