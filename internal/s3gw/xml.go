package s3gw

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"
)

// S3 error codes and their HTTP statuses.
const (
	codeNoSuchBucket       = "NoSuchBucket"
	codeNoSuchKey          = "NoSuchKey"
	codeBucketAlreadyOwned = "BucketAlreadyOwnedByYou"
	codeInvalidBucketName  = "InvalidBucketName"
	codeInvalidRequest     = "InvalidRequest"
	codeInvalidPart        = "InvalidPart"
	codeNoSuchUpload       = "NoSuchUpload"
	codeMalformedXML       = "MalformedXML"
	codeEntityTooLarge     = "EntityTooLarge"
	codeAccessDenied       = "AccessDenied"
	codeInvalidAccessKey   = "InvalidAccessKeyId"
	codeSignatureMismatch  = "SignatureDoesNotMatch"
	codeInternalError      = "InternalError"
)

type s3ErrorResult struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId"`
	HostID    string   `xml:"HostId"`
}

type s3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type s3BucketInfo struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type listAllMyBucketsResult struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`
	Owner   s3Owner  `xml:"Owner"`
	Buckets struct {
		Bucket []s3BucketInfo `xml:"Bucket"`
	} `xml:"Buckets"`
}

type s3Object struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type s3CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

type listBucketResult struct {
	XMLName               xml.Name         `xml:"ListBucketResult"`
	Name                  string           `xml:"Name"`
	Prefix                string           `xml:"Prefix"`
	Delimiter             string           `xml:"Delimiter,omitempty"`
	Marker                string           `xml:"Marker,omitempty"`
	NextMarker            string           `xml:"NextMarker,omitempty"`
	MaxKeys               int              `xml:"MaxKeys"`
	IsTruncated           bool             `xml:"IsTruncated"`
	KeyCount              int              `xml:"KeyCount,omitempty"`
	EncodingType          string           `xml:"EncodingType,omitempty"`
	ContinuationToken     string           `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string           `xml:"NextContinuationToken,omitempty"`
	Contents              []s3Object       `xml:"Contents"`
	CommonPrefixes        []s3CommonPrefix `xml:"CommonPrefixes"`
}

type initiateMultipartResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type completePartRequest struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type completeMultipartRequest struct {
	XMLName xml.Name              `xml:"CompleteMultipartUpload"`
	Parts   []completePartRequest `xml:"Part"`
}

type completeMultipartResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type listMultipartUploadsResult struct {
	XMLName            xml.Name `xml:"ListMultipartUploadsResult"`
	Bucket             string   `xml:"Bucket"`
	KeyMarker          string   `xml:"KeyMarker"`
	UploadIDMarker     string   `xml:"UploadIdMarker"`
	NextKeyMarker      string   `xml:"NextKeyMarker"`
	NextUploadIDMarker string   `xml:"NextUploadIdMarker"`
	MaxUploads         int      `xml:"MaxUploads"`
	IsTruncated        bool     `xml:"IsTruncated"`
}

type locationConstraintResult struct {
	XMLName xml.Name `xml:"LocationConstraint"`
	Region  string   `xml:",chardata"`
}

func writeXML(w http.ResponseWriter, status int, v any) {
	buf, err := xml.Marshal(v)
	if err != nil {
		writeS3Error(w, nil, http.StatusInternalServerError, codeInternalError, "encode response: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n"))
	_, _ = w.Write(buf)
}

// writeS3Error renders the S3 XML error shape. HEAD responses carry no body.
func writeS3Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if r != nil && r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		return
	}
	resource := ""
	if r != nil {
		resource = r.URL.Path
	}
	writeXML(w, status, s3ErrorResult{
		Code:      code,
		Message:   strings.TrimSpace(message),
		Resource:  resource,
		RequestID: "nimbus",
		HostID:    "nimbus",
	})
}

func iso8601(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
