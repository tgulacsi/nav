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

package main

import (
	"bytes"
	"flag"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/tgulacsi/go/text"
	"golang.org/x/net/html"
	"gopkg.in/inconshreveable/log15.v2"
)

var Log = log15.New()

func main() {
	Log.SetHandler(log15.StderrHandler)
	flagVerbose := flag.Bool("v", false, "verbose logging")
	flagURL := flag.String("url", "http://nav.gov.hu/nav/adatbazisok/adatbleker/afaalanyok/afaalanyok_csoportos", "starting URL")
	flag.Parse()
	hndl := log15.StderrHandler
	if !*flagVerbose {
		hndl = log15.LvlFilterHandler(log15.LvlInfo, log15.StderrHandler)
	}
	Log.SetHandler(hndl)

	base, err := getBase(*flagURL)
	if err != nil {
		Log.Error("getBase", "url", *flagURL, "error", err)
		os.Exit(2)
	}
	Log.Info("base", "url", base)

	if err = lekerdez(*flagURL, flag.Args()...); err != nil {
		Log.Error("lekerdez", "url", base, "error", err)
		os.Exit(2)
	}
}
func lekerdez(page string, adoszam ...string) error {
	base, err := getBase(page)
	if err != nil {
		Log.Error("getBase", "url", page, "error", err)
		return err
	}
	Log.Info("base", "url", base)

	params, err := getFormParams(base)
	if err != nil {
		return err
	}
	Log.Debug("lekerdez", "params", params)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, vv := range params.fields {
		pw, err := w.CreateFormField(k)
		if err != nil {
			return err
		}
		for _, v := range vv {
			if _, err = io.WriteString(pw, v); err != nil {
				return err
			}
		}
	}
	pw, err := w.CreateFormFile(params.fileName, "adoszamok.txt")
	if err != nil {
		return err
	}
	if len(adoszam) > 0 {
		Log.Info("Asking", "adoszam", adoszam)
		for _, tn := range adoszam {
			if _, err = io.WriteString(pw, tn[:8]+"\n"); err != nil {
				return err
			}
		}
	} else {
		Log.Info("Copying stdin...")
		if _, err = io.Copy(pw, os.Stdin); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", base, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	page, err = getDownloadURL(resp.Request.URL, resp.Body)
	if err != nil {
		Log.Error("getDownloadURL", "error", err)
		return err
	}
	//http://80.249.172.60/cgi-bin/afaalany/ktmp/2015090310172791EC26C5_catv_pool_telekom_hutempfile.txt:
	Log.Info("get", "url", page)
	resp, err = http.Get(page)
	if err != nil {
		Log.Error("get", "url", page, "error", err)
		return err
	}
	defer resp.Body.Close()

	Log.Info("response", "header", resp.Header)
	var enc string
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil {
		enc = params["charset"]
	}
	if enc == "" {
		enc = "iso-8859-2"
	}
	_, err = io.Copy(os.Stdout, text.NewReader(resp.Body, text.GetEncoding(enc)))
	return err
}

func getDownloadURL(relTo *url.URL, r io.Reader) (string, error) {
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
				return relTo.ResolveReference(nxt).String(), nil
			}
			if !more {
				break
			}
		}
	}
	return "", nil
}

func getBase(page string) (string, error) {
	resp, err := http.Get(page)
	if err != nil {
		Log.Error("get start page", "url", page, "error", err)
		return "", err
	}
	defer resp.Body.Close()
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
	return "", nil
}

type formParams struct {
	fields   map[string][]string
	fileName string
}

func getFormParams(base string) (formParams, error) {
	var params formParams
	resp, err := http.Get(base)
	if err != nil {
		return params, err
	}
	defer resp.Body.Close()

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
