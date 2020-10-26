package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/disintegration/imaging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ImageMessage defines the JSON schema for messages received from SQS
type ImageMessage struct {
	FileID        string `json:"file_id"`
	FileExtension string `json:"file_extension"`
	Directory     string `json:"directory"`
	CallbackURL   string `json:"callback_url"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
}

// CallbackMessage defines the JSON schema for messages sent to SQS after processing
type CallbackMessage struct {
	CallbackURL   string `json:"callback_url"`
	Bucket        string `json:"bucket"`
	Directory     string `json:"directory"`
	FileID        string `json:"file_id"`
	FileExtension string `json:"file_extension"`
}

// validImageFormats defines valid image mime types for processing
var validImageFormats []string = []string{
	"image/png",
	"image/jpeg",
}

var logger *zap.SugaredLogger

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// get environment parameters
	uploadBucket := os.Getenv("AWS_S3_BUCKET_UPLOAD")
	publicBucket := os.Getenv("AWS_S3_BUCKET_PUBLIC")
	callbackQueue := os.Getenv("CALLBACK_QUEUE")
	maxBytes, err := strconv.ParseInt(os.Getenv("MAX_BYTES"), 10, 64)
	if err != nil {
		logger.Errorf("Could not convert MAX_BYTES to int64: %v", err)
		return err
	}
	maxWidth, err := strconv.Atoi(os.Getenv("MAX_WIDTH"))
	if err != nil {
		logger.Errorf("Could not convert MAX_WIDTH to int: %v", err)
		return err
	}
	maxHeight, err := strconv.Atoi(os.Getenv("MAX_HEIGHT"))
	if err != nil {
		logger.Errorf("Could not convert MAX_HEIGHT to int: %v", err)
		return err
	}

	// initialize AWS session
	sess := session.Must(session.NewSession())

	// loop over messages from queue
	for _, message := range sqsEvent.Records {

		logger.Infow("Message parameters:",
			"message_id", message.MessageId,
			"event_source", message.EventSource,
			"body", message.Body,
		)

		// decode message body JSON
		var msg ImageMessage
		err := json.Unmarshal([]byte(message.Body), &msg)
		if err != nil {
			logger.Errorf("Error unmarshalling message body: %v", err)
			continue
		}

		logger.Infow("Message body",
			"file_id", msg.FileID,
			"file_extension", msg.FileExtension,
			"directory", msg.Directory,
			"callback_url", msg.CallbackURL,
			"width", msg.Width,
			"height", msg.Height,
		)

		// simple sanity check
		if msg.FileID == "" || msg.FileExtension == "" {
			logger.Errorf("Missing parameters, cannot complete request; file_id: %s, file_extension: %s", msg.FileID, msg.FileExtension)
			continue
		}

		// assign file names
		var fileKey string
		if msg.Directory != "" {
			fileKey = fmt.Sprintf("%s/%s.%s", msg.Directory, msg.FileID, msg.FileExtension)
		} else {
			fileKey = fmt.Sprintf("%s.%s", msg.FileID, msg.FileExtension)
		}
		localFile := fmt.Sprintf("/tmp/%s.%s", msg.FileID, msg.FileExtension)

		// create local temp file
		file, err := os.Create(localFile)
		if err != nil {
			logger.Errorf("os.Create() error: %s", err)
			continue
		}

		// download file from S3
		numBytes, err := downloadFile(sess, file, uploadBucket, fileKey)
		if err != nil {
			logger.Errorf("S3 downloader error: %s", err)
			close(file)
			continue
		}

		// detect file type
		fileType, err := getFileType(file)
		if err != nil {
			logger.Errorf("File read error: %s", err)
			close(file)
			continue
		}

		// reject bad file types
		if !contains(validImageFormats, fileType) {
			logger.Errorf("Unsupported file type: %s, %s", fileType, fileKey)
			close(file)
			continue
		}

		// reject large files
		if numBytes > maxBytes {
			logger.Errorf("File is too large: %d, %s", numBytes, fileKey)
			close(file)
			continue
		}

		// open image
		img, err := imaging.Open(localFile)
		if err != nil {
			logger.Errorf("Failed to open image: %v", err)
			close(file)
			continue
		}

		// resize image if too large
		newMaxWidth := maxWidth
		if msg.Width > 0 {
			newMaxWidth = min(newMaxWidth, msg.Width)
		}
		newMaxHeight := maxHeight
		if msg.Height > 0 {
			newMaxHeight = min(newMaxHeight, msg.Height)
		}
		err = resizeImageIfTooLarge(img, localFile, newMaxWidth, newMaxHeight)
		if err != nil {
			logger.Errorf("Failed to resize image: %v", err)
			close(file)
			continue
		}

		// upload to public bucket
		err = uploadFile(sess, file, publicBucket, fileKey, fileType)
		if err != nil {
			logger.Errorf("Failed to upload file: %v", err)
			close(file)
			continue
		}

		logger.Infow("Image upload complete.",
			"bucket", publicBucket,
			"file_key", fileKey,
		)

		close(file)

		// send message to callback queue
		callbackMsg := &CallbackMessage{
			CallbackURL:   msg.CallbackURL,
			Bucket:        publicBucket,
			Directory:     msg.Directory,
			FileID:        msg.FileID,
			FileExtension: msg.FileExtension,
		}
		err = sendCallbackMessage(sess, callbackQueue, callbackMsg)
		if err != nil {
			logger.Errorf("Failed send callback message to queue: %v", err)
			continue
		}
	}

	// complete
	return nil
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

// resizeImageIfTooLarge resizes an image if the width or height dimensions are too large
func resizeImageIfTooLarge(img image.Image, localFile string, maxWidth, maxHeight int) error {
	var err error

	// get dimensions
	width := img.Bounds().Max.X
	height := img.Bounds().Max.Y

	// resize if needed
	if width > maxWidth || height > maxHeight {

		ratioX := float64(maxWidth) / float64(width)
		ratioY := float64(maxHeight) / float64(height)
		ratio := math.Min(ratioX, ratioY)

		newWidth := int(float64(width) * ratio)
		newHeight := int(float64(height) * ratio)

		img = imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)
		err = imaging.Save(img, localFile)
	}
	return err
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

// sendCallbackMessage sends an SQS message to the callback queue
func sendCallbackMessage(sess *session.Session, queue string, msg *CallbackMessage) error {

	// marshal the message to JSON
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// create aws session
	svc := sqs.New(sess)

	// get queue URL
	result, err := svc.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: &queue,
	})
	if err != nil {
		return err
	}

	// send message to queue
	_, err = svc.SendMessage(&sqs.SendMessageInput{
		MessageBody: aws.String(string(body)),
		QueueUrl:    result.QueueUrl,
	})
	return err
}

func main() {
	lambda.Start(Handler)
}
