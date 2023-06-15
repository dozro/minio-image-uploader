package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/scottleedavis/go-exif-remove"
)

func getURL(dns string, bucket string, filename string, includeBucketName bool) string {
	retVal := ""
	if includeBucketName {
		retVal = "https://" + dns + "/" + bucket + "/" + filename
	} else {
		retVal = "https://" + dns + "/" + filename
	}

	return retVal
}

func checkIfFileIsValid(file *os.File) bool {
	fileStat, err := file.Stat()
	if err != nil {
		return false
	} else if fileStat.Size() == 0 {
		return false
	} else {
		return true
	}
}

func hashFile(file os.File) string {
	f, _ := os.Open(file.Name())
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func getContentTypeOfFileFromFileName(file *os.File) string {
	contentType := "application/octet-stream"
	fileName := file.Name()
	if strings.HasSuffix(fileName, ".jpg") || strings.HasSuffix(fileName, ".jpeg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(fileName, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(fileName, ".gif") {
		contentType = "image/gif"
	} else if strings.HasSuffix(fileName, ".tif") || strings.HasSuffix(fileName, ".tiff") {
		contentType = "image/tiff"
	} else if strings.HasSuffix(fileName, ".txt") {
		contentType = "text/plain"
	} else if strings.HasSuffix(fileName, ".webp") {
		contentType = "image/webp"
	} else if strings.HasSuffix(fileName, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(fileName, ".ico") {
		contentType = "image/vnd.microsoft.icon"
	}
	return contentType
}

func main() {
	err := godotenv.Load(os.Getenv("HOME") + "/.config/minio-image-uploader.env")
	if err != nil {
		log.Fatal(err)
		return
	}
	// read command line flags
	sourcePathFlag := flag.String("src", "", "the source file you want to upload")
	destPrefixFlag := flag.String("prefix", "", "the destination prefix")
	endpoint := flag.String("endpoint", os.Getenv("S3_ENDPOINT"), "the S3 endpoint")
	accessKey := flag.String("access-key", os.Getenv("S3_ACCESSKEY"), "the access key")
	bucketName := flag.String("bucket", os.Getenv("S3_BUCKETNAME"), "the name of the S3 Bucket")
	secretKey := flag.String("secret-key", os.Getenv("S3_SECRETKEY"), "secret key")
	accessDns := flag.String("access-dns", *endpoint, "the dns")
	accessDnsNoBucketName := flag.Bool("no-access-dns-bucket-name", false, "WIP")
	printURL := flag.Bool("print-url", false, "print the url to access this object")
	getShareURL := flag.Bool("share-url", false, "get the share url")
	debugOutput := flag.Bool("verbose", false, "if you want to get verbose output")
	flag.Parse()
	if len(*sourcePathFlag) == 0 {
		println("please provide the path to the source file using the \"-src\" flag")
		return
	}
	imagePath := *sourcePathFlag
	if len(*destPrefixFlag) == 0 {
		println("please provide a prefix to be prepended when uploading using the \"-prefix\" flag")
		return
	}
	if len(*endpoint) == 0 {
		println("Endpoint is not definied. it was neither defined as environment variable nor as cmdline (\"-endpoint s3.example.org\") flag")
		return
	}
	if len(*accessKey) == 0 {
		println("Access key is not definied. it was neither defined as environment variable nor as cmdline flag")
		return
	}
	if len(*bucketName) == 0 {
		println("Bucket name is not definied. it was neither defined as environment variable nor as cmdline flag (\"-bucket my-bucket\")")
		return
	}
	prefix := *destPrefixFlag

	// Open the image file.
	if *debugOutput {
		// debug output
		log.Println("opening file: " + imagePath)
	}
	file, err := os.Open(imagePath)
	filename := filepath.Base(imagePath)
	if err != nil {
		log.Fatal(err)
	}
	if *debugOutput {
		fileStat, err := file.Stat()
		if err != nil {
			log.Panic(err)
		}
		log.Printf("opened file has the size of %d\n", fileStat.Size())
	}
	defer file.Close()

	// Save the image to a temporary file.
	tempFile, err := ioutil.TempFile("", "")
	if err != nil {
		log.Fatal(err)
	}
	bytes, err := ioutil.ReadFile(file.Name())
	if *debugOutput {
		log.Println("trying to clean exif data from file")
	}
	cleanBytes, err := exifremove.Remove(bytes)
	_, err = tempFile.Write(cleanBytes)
	if err != nil {
		log.Panic(err)
	}
	if !checkIfFileIsValid(tempFile) {
		log.Println("cleaned file is invalid. Falling back to uncleaned file")
		_, err = tempFile.Write(bytes)
		if err != nil {
			log.Panic(err)
		}
	}

	// Upload the image to Minio.
	useSSL := true
	if os.Getenv("S3_USESSL") == "false" {
		useSSL = false
	}
	objectName := prefix + "/" + hashFile(*file) + "/" + filename

	// Initialize the Minio client.
	minioClient, err := minio.New(*endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKey, *secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Set the content type of the object based on its file extension.

	contentType := getContentTypeOfFileFromFileName(tempFile)
	UserTags := make(map[string]string)
	UserTags["agent"] = "Ryes-Minio-Image-Uploader"
	if *getShareURL {
		UserTags["generatedWithShareURL"] = "true"
	}
	// Upload the object to Minio.
	_, err = minioClient.FPutObject(context.Background(), *bucketName, objectName, tempFile.Name(), minio.PutObjectOptions{
		ContentType: contentType,
		UserTags:    UserTags,
	})
	if err != nil {
		log.Fatal(err)
	}
	if *printURL {
		fmt.Println(getURL(*accessDns, *bucketName, objectName, !*accessDnsNoBucketName))
	} else if *getShareURL {
		shareURL, _ := minioClient.PresignedGetObject(context.Background(), *bucketName, objectName, time.Duration(24*time.Hour), nil)
		fmt.Println(shareURL)
	} else {
		fmt.Printf("Uploaded %s to %s/%s\n", imagePath, *bucketName, objectName)
	}
	err = tempFile.Close()
	if err != nil {
		log.Panic(err)
	}
	defer os.Remove(tempFile.Name())
	err = file.Close()
	if err != nil {
		log.Panic(err)
	}
}
