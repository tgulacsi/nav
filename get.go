/*
Copyright 2015 Tamás Gulácsi

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nav

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/inconshreveable/log15.v2"

	"github.com/tgulacsi/go/text"
	"golang.org/x/net/context"
	"golang.org/x/net/html"
)

var Log = log15.New("lib", "nav")

func init() {
	Log.SetHandler(log15.DiscardHandler())
}

type Endpoint struct {
	URL       string
	BatchSize int

	client        *http.Client
	params        formParams
	getParamsOnce sync.Once
}

type Result struct {
	TaxNo string
	Valid bool
	Exist bool
	Owner string
}

const (
	MaxRecordCount   = (200 << 10) / (8 + 1) // 200kB, each record is 8+1 bytes
	DefaultBatchSize = 128
)

var (
	ErrTooManyRecords = errgo.Newf("too many records, max %d.", MaxRecordCount)
)

func (ep *Endpoint) GetFromReader(ctx context.Context, dest chan<- []Result, r io.Reader) error {
	defer close(dest)
	errCh := make(chan error, cap(dest))
	errs := make([]error, 0, cap(dest))
	var errWg sync.WaitGroup
	errWg.Add(1)
	go func() {
		defer errWg.Done()
		for err := range errCh {
			if err != nil {
				errs = append(errs, err)
			}
		}
	}()

	freeChunks := make(chan []string, cap(dest))
	chunks := make(chan []string, cap(dest))
	var wg sync.WaitGroup
	for i := 0; i < cap(chunks); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range chunks {
				result, err := ep.Get(ctx, chunk)
				select {
				case freeChunks <- chunk[:0]:
				default:
				}
				if err != nil {
					errCh <- err
					continue
				}
				dest <- result
			}
		}()
	}

	chunk := make([]string, 0, ep.BatchSize)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		chunk = append(chunk, scanner.Text())
		if len(chunk) == cap(chunk) {
			chunks <- chunk
			select {
			case chunk = <-freeChunks:
				chunk = chunk[:0]
			default:
				chunk = make([]string, 0, ep.BatchSize)
			}
		}
	}
	if len(chunk) > 0 {
		chunks <- chunk
	}
	errCh <- scanner.Err()
	close(chunks)
	wg.Wait()
	close(errCh)
	errWg.Wait()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (ep *Endpoint) Get(ctx context.Context, adoszam []string) ([]Result, error) {
	result := make([]Result, 0, len(adoszam))
	for i, num := range adoszam {
		if num != "" {
			if IsValid(num) {
				continue
			}
			result = append(result, Result{TaxNo: num})
		}
		if i < len(adoszam)-1 {
			adoszam[i] = adoszam[len(adoszam)-1]
		}
		adoszam = adoszam[:len(adoszam)-1]
	}
	if len(adoszam) == 0 {
		return result, nil
	}
	if len(adoszam) > MaxRecordCount {
		return result, errgo.Notef(ErrTooManyRecords, "len=%d", len(adoszam))
	}

	var getParamsErr error
	ep.getParamsOnce.Do(func() {
		if ep.BatchSize == 0 || ep.BatchSize > MaxRecordCount {
			ep.BatchSize = DefaultBatchSize
		}
		tr := *(http.DefaultTransport.(*http.Transport))
		if dl, ok := ctx.Deadline(); ok {
			tr.ResponseHeaderTimeout = dl.Sub(time.Now())
		}
		ep.client = &http.Client{Transport: &tr}
		base, err := getBase(ctx, ep.URL, ep.client)
		if err != nil {
			Log.Error("getBase", "url", ep.URL, "error", err)
			getParamsErr = errgo.Notef(err, "GET "+ep.URL)
			return
		}
		Log.Debug("base", "url", base)

		if ep.params, err = getFormParams(ctx, base, ep.client); err != nil {
			getParamsErr = errgo.Notef(err, "read form params")
			return
		}
		Log.Info("get", "params", ep.params)
	})
	Log.Debug("ep.URL", "url", ep.URL)
	if getParamsErr != nil {
		return result, getParamsErr
	}
	params := ep.params
	if params.fileName == "" {
		panic(params)
	}
	Log.Debug("params", "params", params)

	page, err := uploadFile(ctx, adoszam, params, ep.client)
	if err != nil {
		return result, err
	}

	return downloadResults(ctx, result, page, ep.client)
}

func uploadFile(ctx context.Context, adoszam []string, params formParams, client *http.Client) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, vv := range params.fields {
		pw, err := w.CreateFormField(k)
		if err != nil {
			return "", err
		}
		ew := errWriter{pw, &err}
		for _, v := range vv {
			io.WriteString(ew, v)
		}
		if err != nil {
			return "", err
		}
	}
	pw, err := w.CreateFormFile(params.fileName, "adoszamok.txt")
	if err != nil {
		return "", err
	}
	ew := errWriter{pw, &err}
	for _, tn := range adoszam {
		io.WriteString(ew, tn[:8]+"\n")
	}
	if err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	if !strings.HasSuffix(params.URL, ".php") {
		panic("bad ep.URL: " + params.URL)
	}
	req, err := http.NewRequest("POST", params.URL, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errgo.Notef(err, "POST "+params.URL)
	}
	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		return "", errgo.New("POST " + params.URL + ": " + resp.Status)
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	return findDownloadURL(*resp.Request.URL, resp.Body)
}

func downloadResults(ctx context.Context, result []Result, page string, client *http.Client) ([]Result, error) {

	//http://80.249.172.60/cgi-bin/afaalany/ktmp/2015090310172791EC26C5_catv_pool_telekom_hutempfile.txt:
	Log.Debug("get", "url", page)
	resp, err := client.Get(page)
	if err != nil {
		Log.Error("get", "url", page, "error", err)
		return result, errgo.Notef(err, "GET "+page)
	}
	defer resp.Body.Close()

	select {
	case <-ctx.Done():
		return result, ctx.Err()
	default:
	}

	Log.Debug("response", "header", resp.Header)
	var enc string
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil {
		enc = params["charset"]
	}
	if enc == "" {
		enc = "iso-8859-2"
	}

	scanner := bufio.NewScanner(text.NewReader(resp.Body, text.GetEncoding(enc)))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		i := bytes.IndexByte(line, ';')
		if i < 0 {
			continue
		}
		taxNo := bytes.TrimRight(line[:i], " ")
		line = line[i+1:]
		if i = bytes.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}

		result = append(result, Result{
			TaxNo: string(taxNo),
			Owner: string(line),
			Valid: true,
			Exist: len(taxNo) == 11,
		})
	}
	Log.Info("Download finished", "result.count", len(result))
	return result, err
}

func findDownloadURL(relTo url.URL, r io.Reader) (string, error) {
	z := html.NewTokenizer(r)
Outer:
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			Log.Error("parse page", "error", z.Err())
			return "", z.Err()
		}
		if tt != html.StartTagToken {
			continue
		}
		tn, hasAttr := z.TagName()
		if !(hasAttr && bytes.EqualFold(tn, []byte("input"))) {
			continue
		}
		var onClick string
		for {
			key, val, more := z.TagAttr()
			if bytes.EqualFold(key, []byte("name")) && !bytes.Equal(val, []byte("letolt")) {
				continue Outer
			}
			if bytes.EqualFold(key, []byte("onclick")) {
				onClick = string(val)
				if i := strings.IndexAny(onClick, `'"`); i >= 0 {
					sep := onClick[i]
					onClick = onClick[i+1:]
					if i = strings.LastIndex(onClick, string(sep)); i >= 0 {
						onClick = onClick[:i]
					}
				}
				nxt, err := url.Parse(onClick)
				if err != nil {
					return onClick, err
				}
				relTo := &relTo
				return relTo.ResolveReference(nxt).String(), nil
			}
			if !more {
				break
			}
		}
	}
	return "", errors.New("no download URL found")
}

func getBase(ctx context.Context, page string, client *http.Client) (string, error) {
	resp, err := client.Get(page)
	if err != nil {
		Log.Error("get start page", "url", page, "error", err)
		return "", err
	}
	defer resp.Body.Close()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			Log.Error("parse page", "error", z.Err())
			return "", z.Err()
		}
		if tt != html.StartTagToken {
			continue
		}
		tn, hasAttr := z.TagName()
		if !(hasAttr && bytes.EqualFold(tn, []byte("iframe"))) {
			continue
		}
		for {
			key, val, more := z.TagAttr()
			if bytes.EqualFold(key, []byte("src")) {
				return string(val), nil
			}
			if !more {
				break
			}
		}
	}
	return "", errgo.Notef(errgo.New("no iframe found"), "page="+page)
}

type formParams struct {
	URL      string
	fields   map[string][]string
	fileName string
}

func getFormParams(ctx context.Context, base string, client *http.Client) (formParams, error) {
	params := formParams{URL: base}
	resp, err := client.Get(base)
	if err != nil {
		return params, err
	}
	defer resp.Body.Close()
	select {
	case <-ctx.Done():
		return params, ctx.Err()
	default:
	}

	params.fields = make(map[string][]string, 4)
	z := html.NewTokenizer(resp.Body)
	state := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			Log.Error("parse page", "error", z.Err())
			return params, z.Err()
		}
		if tt != html.StartTagToken {
			continue
		}
		tn, hasAttr := z.TagName()
		if state == 0 {
			if !bytes.EqualFold(tn, []byte("form")) {
				continue
			}
			state++
			continue
		}
		if !(hasAttr && bytes.EqualFold(tn, []byte("input"))) {
			continue
		}
		var name, value, typ string
		for {
			key, val, more := z.TagAttr()
			switch {
			case bytes.EqualFold(key, []byte("name")):
				name = string(val)
			case bytes.EqualFold(key, []byte("type")):
				typ = string(bytes.ToLower(val))
			case bytes.EqualFold(key, []byte("value")):
				value = string(val)
			}
			if !more {
				break
			}
		}
		if typ == "file" {
			params.fileName = name
		} else {
			params.fields[name] = append(params.fields[name], value)
		}
	}

	return params, nil
}

type errWriter struct {
	w    io.Writer
	errp *error
}

func (ew errWriter) Write(p []byte) (int, error) {
	err := *ew.errp
	if err != nil {
		return 0, err
	}
	var n int
	if n, err = ew.w.Write(p); err != nil {
		*ew.errp = err
	}
	return n, err
}
