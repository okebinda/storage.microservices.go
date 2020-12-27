package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	chiproxy "github.com/awslabs/aws-lambda-go-api-proxy/chi"
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger
var adapter *chiproxy.ChiLambda

// validImageFormats defines valid image mime types for processing
var validImageFormats []string = []string{
	"image/png",
	"image/jpeg",
}

func init() {
	r := chi.NewRouter()

	r.Get("/ratio/{size}/*", GetResizeRatio)
	r.Get("/crop/{size}/*", GetResizeCrop)

	adapter = chiproxy.New(r)
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// serve request
	c, err := adapter.ProxyWithContext(ctx, request)
	return c, err
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
func redirectResponse(w http.ResponseWriter, r *http.Request, redirectURL string) {
	http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
}

// userErrorResponse generates a user error (400) response
func userErrorResponse(w http.ResponseWriter, code int, errorMessage string) {
	body, err := json.Marshal(map[string]interface{}{
		"error": errorMessage,
	})
	if err != nil {
		logger.Errorf("Marshalling error: %s", err)
		serverErrorResponse(w)
	}
	generateResponse(w, code, body)
}

// serverErrorResponse generates a server error (500) response
func serverErrorResponse(w http.ResponseWriter) {
	generateResponse(w, 500, []byte("{\"error\":\"Server error\"}"))
}

// generateResponse generates an HTTP JSON Lambda response to return to the user
func generateResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, err := w.Write(body)
	if err != nil {
		logger.Errorf("Error writing response: %s", err)
	}
}

func main() {
	lambda.Start(Handler)
}
