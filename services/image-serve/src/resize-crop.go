package main

import (
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/disintegration/imaging"
	"github.com/go-chi/chi"
)

// GetResizeCrop resizes an image and saves to an S3 bucket, cropping to fit the given dimensions
func GetResizeCrop(w http.ResponseWriter, r *http.Request) {

	// get environment parameters
	sourceBucket := os.Getenv("AWS_S3_BUCKET_SOURCE")
	destinationBucket := os.Getenv("AWS_S3_BUCKET_DESTINATION")
	region := os.Getenv("REGION")
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

	// get path parameters
	size := chi.URLParam(r, "size")

	// get path parameters (chi doesn't support greedy path parameters)
	rePath := regexp.MustCompile(`^/crop/\d+x\d+/`)
	imageKey := rePath.ReplaceAllString(r.RequestURI, "")

	logger.Infow("Request parameters",
		"size", size,
		"imageKey", imageKey,
	)

	// simple sanity check
	if size == "" || imageKey == "" {
		errorMessage := fmt.Sprintf("Missing parameters, cannot complete request; size: %s, image_key: %s", size, imageKey)
		logger.Error(errorMessage)
		userErrorResponse(w, 400, errorMessage)
		return
	}

	// check size parameter is correct format
	isMatch, err := regexp.MatchString(`^\d+x\d+$`, size)
	if err != nil {
		errorMessage := fmt.Sprintf("Could not read parameter format, cannot complete request; size: %s: %v", size, err)
		logger.Error(errorMessage)
		userErrorResponse(w, 400, errorMessage)
		return
	}
	if isMatch == false {
		errorMessage := fmt.Sprintf("Bad parameter format, cannot complete request; size: %s", size)
		logger.Error(errorMessage)
		userErrorResponse(w, 400, errorMessage)
		return
	}

	// parse image dimensions from path
	sizes := strings.Split(size, "x")
	width, err := strconv.Atoi(sizes[0])
	if err != nil {
		logger.Errorf("Could not convert sizes[0] to int: %v", err)
		userErrorResponse(w, 400, "Could not convert width to int.")
		return
	}
	height, err := strconv.Atoi(sizes[1])
	if err != nil {
		logger.Errorf("Could not convert sizes[1] to int: %v", err)
		userErrorResponse(w, 400, "Could not convert height to int.")
		return
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
		serverErrorResponse(w)
		return
	}

	// download file from S3
	_, err = downloadFile(sess, file, sourceBucket, imageKey)
	if err != nil {
		logger.Errorf("S3 downloader error: %s, %s", imageKey, err)
		close(file)
		if strings.HasPrefix(err.Error(), "NoSuchKey") {
			userErrorResponse(w, 404, "Not found.")
			return
		}
		serverErrorResponse(w)
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
		errorMessage := fmt.Sprintf("Unsupported file type: %s", fileType)
		logger.Error(errorMessage)
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

	// resize image
	width = min(maxWidth, width)
	height = min(maxHeight, height)
	err = resizeImageCrop(img, localFile, width, height)
	if err != nil {
		logger.Errorf("Failed to resize image: %v", err)
		close(file)
		serverErrorResponse(w)
		return
	}

	// upload to public bucket
	err = uploadFile(sess, file, destinationBucket, resizedFileKey, fileType)
	if err != nil {
		logger.Errorf("Failed to upload file: %s, %v", resizedFileKey, err)
		close(file)
		serverErrorResponse(w)
		return
	}

	logger.Infow("Image resize complete.",
		"bucket", destinationBucket,
		"file_key", resizedFileKey,
		"width", width,
		"height", height,
	)

	close(file)

	// response
	redirectURL := fmt.Sprintf("http://%s.s3-website.%s.amazonaws.com/%s", destinationBucket, region, resizedFileKey)
	redirectResponse(w, r, redirectURL)
}

// resizeImageCrop resizes an image, cropping to widthxheight
func resizeImageCrop(img image.Image, localFile string, widthIn, heightIn int) error {
	var err error
	img = imaging.Fill(img, widthIn, heightIn, imaging.Center, imaging.Lanczos)
	err = imaging.Save(img, localFile)
	return err
}
