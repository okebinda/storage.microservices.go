package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Response is the response sent to AWS API Gateway
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

var logger *zap.SugaredLogger

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (Response, error) {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// get environment parameters
	bucket := os.Getenv("AWS_S3_BUCKET_PUBLIC")

	// get path parameters
	imageKey := request.PathParameters["image_key"]

	logger.Infow("Request parameters",
		"imageKey", imageKey,
	)

	// simple sanity check
	if imageKey == "" {
		logger.Errorf("Missing parameters, cannot complete request; image_key: %s", imageKey)
		return userErrorResponse(fmt.Sprintf("Missing parameters, cannot complete request; image_key: %s", imageKey))
	}

	// delete object
	err := deleteObject(bucket, imageKey)
	if err != nil {
		logger.Errorf("Failed delete object: %s", err)
		return serverErrorResponse(err)
	}

	logger.Infow("Object deleted.")

	// response
	return successResponse()
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

// deleteObject deletes a file from an S3 bucket
func deleteObject(bucketName, fileKey string) error {

	// connect to AWS and create an S3 client
	sess := session.Must(session.NewSession())
	svc := s3.New(sess)

	// delete object from bucket
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileKey),
	}
	_, err := svc.DeleteObject(input)
	return err
}

// successResponse generates a success (204) response
func successResponse() (Response, error) {
	var body []byte
	return generateResponse(204, body), nil
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
