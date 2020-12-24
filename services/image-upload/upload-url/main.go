package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Response is the response sent to AWS API Gateway
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// extensionMap maps extensions to mime types
var extensionMap map[string]string = map[string]string{
	"png":  "png",
	"jpg":  "jpeg",
	"jpeg": "jpeg",
}

var logger *zap.SugaredLogger

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (Response, error) {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// get request parameters
	directory := request.QueryStringParameters["directory"]
	extension := request.QueryStringParameters["extension"]

	logger.Infow("Request parameters",
		"directory", directory,
		"extension", extension,
	)

	// basic sanity test for extension
	extensionType, ok := extensionMap[extension]
	if !ok {
		logger.Errorf("Unsupported extension: %s", extension)
		return userErrorResponse(fmt.Sprintf("Unsupported extension: %s", extension))
	}

	// generate S3 file key
	fileKey := generateFileKey(extension, directory)

	// generate a presigned upload URL
	signedURL, err := generatePresignedURL(os.Getenv("AWS_S3_BUCKET_UPLOAD"), fileKey, extensionType, 15)
	if err != nil {
		logger.Errorf("Failed to sign request: %s", err)
		return serverErrorResponse(err)
	}

	logger.Infow("Response parameters",
		"upload_url", signedURL,
		"file_key", fileKey,
	)

	// response
	return successResponse(map[string]interface{}{
		"upload_url": signedURL,
		"file_key":   fileKey,
	})
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

// generateFileKey generates a file key for storage in an S3 bucket
func generateFileKey(extension, directory string) string {
	var fileKey string
	fileID := uuid.New()
	if directory == "" {
		fileKey = fmt.Sprintf("%s.%s", fileID, extension)
	} else {
		fileKey = fmt.Sprintf("%s/%s.%s", directory, fileID, extension)
	}
	return fileKey
}

// generatePresignedURL generates a presigned upload URL for S3 bucket
func generatePresignedURL(bucket, fileKey, extensionType string, expires time.Duration) (string, error) {

	// connect to AWS and create an S3 client
	sess := session.Must(session.NewSession())
	svc := s3.New(sess)

	// generate a presigned upload URL
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(fileKey),
		ContentType: aws.String(fmt.Sprintf("image/%s", extensionType)),
	})
	return req.Presign(expires * time.Minute)
}

// successResponse generates a success (200) response
func successResponse(fields map[string]interface{}) (Response, error) {
	body, err := json.Marshal(fields)
	if err != nil {
		logger.Errorf("Marshalling error: %s", err)
		return Response{StatusCode: 500}, err
	}
	return generateResponse(200, body), nil
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
			"Content-Type": "application/json; charset=utf-8",
		},
	}
}

func main() {
	lambda.Start(Handler)
}
