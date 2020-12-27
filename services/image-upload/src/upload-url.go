package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
)

// extensionMap maps extensions to mime types
var extensionMap map[string]string = map[string]string{
	"png":  "png",
	"jpg":  "jpeg",
	"jpeg": "jpeg",
}

// GetUploadURL retrieves a pre-signed S3 bucket upload URL
func GetUploadURL(w http.ResponseWriter, r *http.Request) {

	// get request parameters
	directory := r.URL.Query().Get("directory")
	extension := r.URL.Query().Get("extension")

	logger.Infow("Request parameters",
		"directory", directory,
		"extension", extension,
	)

	// basic sanity test for extension
	extensionType, ok := extensionMap[extension]
	if !ok {
		logger.Errorf("Unsupported extension: %s", extension)
		userErrorResponse(w, 400, fmt.Sprintf("Unsupported extension: %s", extension))
	}

	// generate S3 file key
	fileKey := generateFileKey(extension, directory)

	// generate a presigned upload URL
	signedURL, err := generatePresignedURL(os.Getenv("AWS_S3_BUCKET_UPLOAD"), fileKey, extensionType, 15)
	if err != nil {
		logger.Errorf("Failed to sign request: %s", err)
		serverErrorResponse(w)
	}

	logger.Infow("Response parameters",
		"upload_url", signedURL,
		"file_key", fileKey,
	)

	// response
	successResponse(w, 200, map[string]interface{}{
		"upload_url": signedURL,
		"file_key":   fileKey,
	})
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
