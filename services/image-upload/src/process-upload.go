package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/disintegration/imaging"
)

// RequestPayload defines the JSON schema for payload received from the request
type RequestPayload struct {
	Directory     string `json:"directory"`
	FileExtension string `json:"file_extension"`
	FileID        string `json:"file_id"`
	Height        int    `json:"height"`
	Width         int    `json:"width"`
}

// ResponsePayload defines the JSON schema for the payload to send to the callback URL
type ResponsePayload struct {
	Bucket        string `json:"bucket"`
	Directory     string `json:"directory"`
	FileExtension string `json:"file_extension"`
	FileID        string `json:"file_id"`
	Height        int    `json:"height"`
	SizeBytes     int64  `json:"size_bytes"`
	Width         int    `json:"width"`
}

// validImageFormats defines valid image mime types for processing
var validImageFormats []string = []string{
	"image/png",
	"image/jpeg",
}

// PostProcessUpload moves an image from the upload S3 bucket to the static S3 bucket
func PostProcessUpload(w http.ResponseWriter, r *http.Request) {

	// get environment parameters
	uploadBucket := os.Getenv("AWS_S3_BUCKET_UPLOAD")
	publicBucket := os.Getenv("AWS_S3_BUCKET_PUBLIC")
	maxBytes, err := strconv.ParseInt(os.Getenv("MAX_BYTES"), 10, 64)
	if err != nil {
		logger.Errorf("Could not convert MAX_BYTES to int64: %v", err)
		serverErrorResponse(w)
		return
	}
	maxWidth, err := strconv.Atoi(os.Getenv("MAX_WIDTH"))
	if err != nil {
		logger.Errorf("Could not convert MAX_WIDTH to int: %v", err)
		serverErrorResponse(w)
		return
	}
	maxHeight, err := strconv.Atoi(os.Getenv("MAX_HEIGHT"))
	if err != nil {
		logger.Errorf("Could not convert MAX_HEIGHT to int: %v", err)
		serverErrorResponse(w)
		return
	}

	// get payload from request body
	var requestData RequestPayload
	decoder := json.NewDecoder(r.Body)
	if err = decoder.Decode(&requestData); err != nil {
		logger.Errorf("Error unmarshalling request body: %v", err)
		serverErrorResponse(w)
		return
	}
	defer r.Body.Close()

	logger.Infow("Request data",
		"directory", requestData.Directory,
		"file_extension", requestData.FileExtension,
		"file_id", requestData.FileID,
		"height", requestData.Height,
		"width", requestData.Width,
	)

	// simple sanity check
	if requestData.FileID == "" || requestData.FileExtension == "" {
		errorMessage := fmt.Sprintf("Missing parameters, cannot complete request; file_id: %s, file_extension: %s", requestData.FileID, requestData.FileExtension)
		logger.Error(errorMessage)
		userErrorResponse(w, 400, errorMessage)
		return
	}

	// assign file names
	var fileKey string
	if requestData.Directory != "" {
		fileKey = fmt.Sprintf("%s/%s.%s", requestData.Directory, requestData.FileID, requestData.FileExtension)
	} else {
		fileKey = fmt.Sprintf("%s.%s", requestData.FileID, requestData.FileExtension)
	}
	localFile := fmt.Sprintf("/tmp/%s.%s", requestData.FileID, requestData.FileExtension)

	// create local temp file
	file, err := os.Create(localFile)
	if err != nil {
		logger.Errorf("os.Create() error: %s", err)
		serverErrorResponse(w)
		return
	}

	// initialize AWS session
	sess := session.Must(session.NewSession())

	// download file from S3
	numBytes, err := downloadFile(sess, file, uploadBucket, fileKey)
	if err != nil {
		logger.Errorf("S3 downloader error: %s", err)
		close(file)
		if strings.HasPrefix(err.Error(), "NoSuchKey") {
			userErrorResponse(w, 404, "Not found.")
			return
		}
		serverErrorResponse(w)
		return
	}

	// reject large files
	if numBytes > maxBytes {
		errorMessage := fmt.Sprintf("File is too large: %d, %s", numBytes, fileKey)
		logger.Errorf(errorMessage)
		close(file)
		userErrorResponse(w, 400, errorMessage)
		return
	}

	// detect file type
	fileType, err := getFileType(file)
	if err != nil {
		logger.Errorf("File read error: %s", err)
		close(file)
		serverErrorResponse(w)
		return
	}

	// reject bad file types
	if !contains(validImageFormats, fileType) {
		errorMessage := fmt.Sprintf("Unsupported file type: %s, %s", fileType, fileKey)
		logger.Errorf(errorMessage)
		close(file)
		userErrorResponse(w, 400, errorMessage)
		return
	}

	// open image
	img, err := imaging.Open(localFile)
	if err != nil {
		logger.Errorf("Failed to open image: %v", err)
		close(file)
		serverErrorResponse(w)
		return
	}

	// resize image if too large
	newMaxWidth := maxWidth
	if requestData.Width > 0 {
		newMaxWidth = min(newMaxWidth, requestData.Width)
	}
	newMaxHeight := maxHeight
	if requestData.Height > 0 {
		newMaxHeight = min(newMaxHeight, requestData.Height)
	}
	finalWidth, finalHeight, err := resizeImageIfTooLarge(img, localFile, newMaxWidth, newMaxHeight)
	if err != nil {
		logger.Errorf("Failed to resize image: %v", err)
		close(file)
		serverErrorResponse(w)
		return
	}

	// upload to public bucket
	err = uploadFile(sess, file, publicBucket, fileKey, fileType)
	if err != nil {
		logger.Errorf("Failed to upload file: %v", err)
		close(file)
		serverErrorResponse(w)
		return
	}

	logger.Infow("Image upload complete.",
		"bucket", publicBucket,
		"file_key", fileKey,
	)

	// get final file size
	fileInfo, err := file.Stat()
	if err != nil {
		logger.Errorf("Failed to stat file: %v", err)
		close(file)
		serverErrorResponse(w)
		return
	}
	finalNumBytes := fileInfo.Size()

	close(file)

	// create response payload
	responseData := &ResponsePayload{
		Bucket:        publicBucket,
		Directory:     requestData.Directory,
		FileExtension: requestData.FileExtension,
		FileID:        requestData.FileID,
		Height:        finalWidth,
		SizeBytes:     finalNumBytes,
		Width:         finalHeight,
	}

	// response
	successResponse(w, 201, responseData)
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
func resizeImageIfTooLarge(img image.Image, localFile string, maxWidth, maxHeight int) (int, int, error) {
	var err error

	// get dimensions
	width := img.Bounds().Max.X
	height := img.Bounds().Max.Y

	// resize if needed
	if width > maxWidth || height > maxHeight {

		ratioX := float64(maxWidth) / float64(width)
		ratioY := float64(maxHeight) / float64(height)
		ratio := math.Min(ratioX, ratioY)

		width = int(float64(width) * ratio)
		height = int(float64(height) * ratio)

		img = imaging.Resize(img, width, height, imaging.Lanczos)
		err = imaging.Save(img, localFile)
	}
	return width, height, err
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
