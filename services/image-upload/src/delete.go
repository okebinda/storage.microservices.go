package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// DeleteImage removes an image from the static S3 bucket
func DeleteImage(w http.ResponseWriter, r *http.Request) {

	// get environment parameters
	bucket := os.Getenv("AWS_S3_BUCKET_PUBLIC")

	// get path parameters (chi doesn't support greedy path parameters)
	imageKey := strings.Replace(r.RequestURI, "/image/delete/", "", 1)

	logger.Infow("Request parameters",
		"imageKey", imageKey,
	)

	// simple sanity check
	if imageKey == "" {
		logger.Errorf("Missing parameters, cannot complete request; image_key: %s", imageKey)
		userErrorResponse(w, 400, fmt.Sprintf("Missing parameters, cannot complete request; image_key: %s", imageKey))
	}

	// delete object
	err := deleteObject(bucket, imageKey)
	if err != nil {
		logger.Errorf("Failed delete object: %s", err)
		serverErrorResponse(w)
	}

	logger.Infow("Object deleted.")

	// response
	successResponse(w, 204, nil)
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
