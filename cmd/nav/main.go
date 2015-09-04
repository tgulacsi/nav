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
	"flag"
	"os"

	"gopkg.in/inconshreveable/log15.v2"
)

var Log = log15.New()

const MaxRecordNum = (200 << 10) / (8 + 1) // 200kB, each record is 8+1 bytes

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
