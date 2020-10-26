package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// CallbackMessage defines the JSON schema for messages received from SQS
type CallbackMessage struct {
	CallbackURL   string `json:"callback_url"`
	Bucket        string `json:"bucket"`
	Directory     string `json:"directory"`
	FileID        string `json:"file_id"`
	FileExtension string `json:"file_extension"`
}

// CallbackPayload defines the JSON schema for the payload to send to the callback URL
type CallbackPayload struct {
	Bucket        string `json:"bucket"`
	Directory     string `json:"directory"`
	FileID        string `json:"file_id"`
	FileExtension string `json:"file_extension"`
}

var logger *zap.SugaredLogger

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {

	// initialize logger
	lc, _ := lambdacontext.FromContext(ctx)
	logger = sugaredLogger(lc.AwsRequestID)
	defer logger.Sync()

	// get environment parameters
	environment := os.Getenv("ENVIRONMENT")
	apiSecretKey := os.Getenv("API_SECRET_KEY")
	apiUsername := os.Getenv("API_USERNAME")
	apiPassword := os.Getenv("API_PASSWORD")

	// loop over messages from queue
	for _, message := range sqsEvent.Records {

		logger.Infow("Message parameters:",
			"message_id", message.MessageId,
			"event_source", message.EventSource,
			"body", message.Body,
		)

		// decode message body JSON
		var msg CallbackMessage
		err := json.Unmarshal([]byte(message.Body), &msg)
		if err != nil {
			logger.Errorf("Error unmarshalling message body: %v", err)
			continue
		}

		logger.Infow("Message body:",
			"callback_url", msg.CallbackURL,
			"bucket", msg.Bucket,
			"file_id", msg.FileID,
			"file_extension", msg.FileExtension,
			"directory", msg.Directory,
		)

		// simple sanity check
		if msg.CallbackURL == "" || msg.Bucket == "" || msg.FileID == "" || msg.FileExtension == "" {
			logger.Errorf("Missing parameters, cannot complete request; callback_url: %s, bucket: %s, file_id: %s, file_extension: %s", msg.CallbackURL, msg.Bucket, msg.FileID, msg.FileExtension)
			continue
		}

		// create callback payload
		payloadJSON := &CallbackPayload{
			Bucket:        msg.Bucket,
			Directory:     msg.Directory,
			FileID:        msg.FileID,
			FileExtension: msg.FileExtension,
		}
		payload, err := json.Marshal(payloadJSON)
		if err != nil {
			logger.Errorf("Error marshalling payload JSON: %v", err)
			continue
		}

		// skip actual callback if testing
		if environment == "TEST" {
			continue
		}

		// perform callback
		req, err := http.NewRequest(http.MethodPost, msg.CallbackURL, bytes.NewBuffer(payload))
		if apiSecretKey != "" {
			req.Header.Add("Authorization", fmt.Sprintf("Apikey %s", apiSecretKey))
		}
		if apiUsername != "" && apiPassword != "" {
			req.SetBasicAuth(apiUsername, apiPassword)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			logger.Errorf("Error calling API: %v", err)
			continue
		}
		defer resp.Body.Close()

		// log response
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)
		logger.Infow("Callback complete.",
			"status", resp.StatusCode,
			"response", bodyString,
		)
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

func main() {
	lambda.Start(Handler)
}
