package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	chiproxy "github.com/awslabs/aws-lambda-go-api-proxy/chi"
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger
var adapter *chiproxy.ChiLambda

func init() {
	r := chi.NewRouter()

	r.Get("/image/upload-url", GetUploadURL)
	r.Post("/image/process-upload", PostProcessUpload)
	r.Delete("/image/delete/*", DeleteImage)

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

// authentication checks the request headers for an X_API_KEY value and compares it to env parameter
func authentication(r *http.Request) bool {
	APIKey := os.Getenv("API_KEY")
	if APIKey != "" {
		headerAPIKey := r.Header.Get("X-API-KEY")
		if headerAPIKey != APIKey {
			return false
		}
	}
	return true
}

// successResponse generates a success (200) response
func successResponse(w http.ResponseWriter, code int, fields interface{}) {
	body, err := json.Marshal(fields)
	if err != nil {
		logger.Errorf("Marshalling error: %s", err)
		serverErrorResponse(w)
	}
	generateResponse(w, code, body)
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
