package general_functions

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	zLog "github.com/rs/zerolog/log"
	"html/template"
	"icapeg/logger"
	"icapeg/service/ContentTypes"
	"icapeg/utils"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type (
	errorPage struct {
		Reason                    string `json:"reason"`
		RequestedURL              string `json:"requested_url"`
		XAdaptationFileId         string `json:"x-adaptation-file-id"`
		XSdkEngineVersion         string `json:"x-sdk-engine-version"`
		XGlasswallCloudApiVersion string `json:"x-glasswall-cloud-api-version"`
	}
)

type GeneralFunc struct {
	httpMsg *utils.HttpMsg
	elapsed time.Duration
	logger  *logger.ZLogger
}

func NewGeneralFunc(httpMsg *utils.HttpMsg, elapsed time.Duration, logger *logger.ZLogger) *GeneralFunc {
	GeneralFunc := &GeneralFunc{
		httpMsg: httpMsg,
		logger:  logger,
		elapsed: elapsed,
	}
	return GeneralFunc
}

func (f *GeneralFunc) CopyingFileToTheBuffer(methodName string) (*bytes.Buffer, ContentTypes.ContentType, error) {
	file := &bytes.Buffer{}
	var err error
	var reqContentType ContentTypes.ContentType
	reqContentType = nil
	switch methodName {
	case utils.ICAPModeReq:
		file, reqContentType, err = f.copyingFileToTheBufferReq()
		break
	case utils.ICAPModeResp:
		file, err = f.copyingFileToTheBufferResp()
		break
	}
	if err != nil {
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Error().Dur("duration", f.elapsed).Err(err).
			Str("value", "Failed to copy the http message body to buffer").Msgf("read_request_body_error")
		return nil, nil, err
	}
	return file, reqContentType, nil
}

func (f *GeneralFunc) copyingFileToTheBufferResp() (*bytes.Buffer, error) {
	file := &bytes.Buffer{}
	_, err := io.Copy(file, f.httpMsg.Response.Body)
	return file, err
}

func (f *GeneralFunc) copyingFileToTheBufferReq() (*bytes.Buffer, ContentTypes.ContentType, error) {
	reqContentType := ContentTypes.GetContentType(f.httpMsg.Request)
	// getting the file from request and store it in buf as a type of bytes.Buffer
	file := reqContentType.GetFileFromRequest()
	return file, reqContentType, nil

}

func (f *GeneralFunc) inStringSlice(data string, ss []string) bool {
	for _, s := range ss {
		if data == s {
			return true
		}
	}
	return false
}

func (f *GeneralFunc) IfFileExtIsBypass(fileExtension string, bypassExts []string) error {
	if utils.InStringSlice(fileExtension, bypassExts) {
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Debug().Dur("duration", f.elapsed).Str("value", fmt.Sprintf("processing not required for file type- %s", fileExtension)).Msgf("belongs_bypassable_extensions")
		return errors.New("processing not required for file type")
	}
	return nil
}

func (f *GeneralFunc) IfFileExtIsBypassAndNotProcess(fileExtension string, bypassExts []string, processExts []string) error {
	if utils.InStringSlice(utils.Any, bypassExts) && !utils.InStringSlice(fileExtension, processExts) {
		// if extension does not belong to "All bypassable except the processable ones" group
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Debug().Dur("duration", f.elapsed).Str("value", fmt.Sprintf("processing not required for file type- %s", fileExtension)).Msgf("dont_belong_to_processable_extensions")
		return errors.New("processing not required for file type")
	}
	return nil
}

func (f *GeneralFunc) IsBodyGzipCompressed(methodName string) bool {
	switch methodName {
	case utils.ICAPModeReq:
		return f.httpMsg.Request.Header.Get("Content-Encoding") == "gzip"
		break
	case utils.ICAPModeResp:
		return f.httpMsg.Response.Header.Get("Content-Encoding") == "gzip"
		break
	}
	return false
}

func (f *GeneralFunc) DecompressGzipBody(file *bytes.Buffer) (*bytes.Buffer, error) {
	reader, _ := gzip.NewReader(file)
	var result []byte
	result, err := ioutil.ReadAll(reader)
	if err != nil {
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Error().Dur("duration", f.elapsed).Err(err).Str("value", "failed to decompress input file").
			Msgf("decompress_gz_file_failed")
		return nil, err
	}
	return bytes.NewBuffer(result), nil
}

func (f *GeneralFunc) IfMaxFileSeizeExc(returnOrigIfMaxSizeExc bool, file *bytes.Buffer, maxFileSize int) (int, *bytes.Buffer, interface{}) {
	zLog.Debug().Dur("duration", f.elapsed).Str("value",
		fmt.Sprintf("file size exceeds max filesize limit %d", maxFileSize)).
		Msgf("large_file_size")
	if returnOrigIfMaxSizeExc {
		return utils.NoModificationStatusCodeStr, file, nil
	} else {
		htmlErrPage := f.GenHtmlPage("service/unprocessable-file.html",
			"The Max file size is exceeded", f.httpMsg.Request.RequestURI)
		f.httpMsg.Response = f.ErrPageResp(http.StatusForbidden, htmlErrPage.Len())
		return utils.OkStatusCodeStr, htmlErrPage, f.httpMsg.Response
	}
}

// GetFileName returns the filename from the http request
func (f *GeneralFunc) GetFileName() string {
	// req.RequestURI  inserting dummy response if the http request is nil
	var Requrl string
	if f.httpMsg.Request == nil || f.httpMsg.Request.RequestURI == "" {
		Requrl = "http://www.example/images/unnamed_file"

	} else {
		Requrl = f.httpMsg.Request.RequestURI
		if Requrl[len(Requrl)-1] == '/' {
			Requrl = Requrl[0 : len(Requrl)-1]
		}

	}
	u, _ := url.Parse(Requrl)

	uu := strings.Split(u.EscapedPath(), "/")

	if len(uu) > 0 {
		return uu[len(uu)-1]
	}
	return "unnamed_file"
}

func (f *GeneralFunc) ExtractFileFromServiceResp(serviceResp *http.Response) ([]byte, error) {
	defer serviceResp.Body.Close()
	bodyByte, err := ioutil.ReadAll(serviceResp.Body)
	if err != nil {
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Error().Dur("duration", f.elapsed).Err(err).Str("value",
			"failed to read the response body from API response").
			Msgf("read_response_body_from_API_error")
		return nil, err
	}
	return bodyByte, nil
}

func (f *GeneralFunc) CompressFileGzip(scannedFile []byte) ([]byte, error) {
	var newBuf bytes.Buffer
	gz := gzip.NewWriter(&newBuf)
	if _, err := gz.Write(scannedFile); err != nil {
		f.elapsed = time.Since(f.logger.LogStartTime)
		zLog.Error().Dur("duration", f.elapsed).Err(err).
			Str("value", "failed to decompress input file").Msgf("decompress_gz_file_failed")
		return nil, err
	}
	gz.Close()
	return newBuf.Bytes(), nil
}

func (f *GeneralFunc) ErrPageResp(status int, pageContentLength int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header: http.Header{
			utils.ContentType:   []string{utils.HTMLContentType},
			utils.ContentLength: []string{strconv.Itoa(pageContentLength)},
		},
	}
}

func (f *GeneralFunc) GenHtmlPage(path, reason, reqUrl string) *bytes.Buffer {
	htmlTmpl, _ := template.ParseFiles(path)
	htmlErrPage := &bytes.Buffer{}
	htmlTmpl.Execute(htmlErrPage, &errorPage{
		Reason:       reason,
		RequestedURL: reqUrl,
	})
	return htmlErrPage
}

func (f *GeneralFunc) PreparingFileAfterScanning(scannedFile []byte, reqContentType ContentTypes.ContentType, methodName string) []byte {
	switch methodName {
	case utils.ICAPModeReq:
		return []byte(reqContentType.BodyAfterScanning(scannedFile))
	}
	return scannedFile
}
