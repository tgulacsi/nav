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
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/tgulacsi/nav"

	"gopkg.in/inconshreveable/log15.v2"
)

var Log = log15.New()

func main() {
	hndl := log15.CallerFileHandler(log15.StderrHandler)
	Log.SetHandler(hndl)
	flagVerbose := flag.Bool("v", false, "verbose logging")
	flagURL := flag.String("url", "http://nav.gov.hu/nav/adatbazisok/adatbleker/afaalanyok/afaalanyok_csoportos", "starting URL")
	flagBatchSize := flag.Int("batch.size", nav.DefaultBatchSize, "batch size")
	flagTimeout := flag.Duration("timeout", 5*time.Minute, "timeout duration")
	flag.Parse()
	if !*flagVerbose {
		hndl = log15.LvlFilterHandler(log15.LvlInfo, log15.StderrHandler)
	}
	Log.SetHandler(hndl)
	nav.Log.SetHandler(hndl)

	var wg sync.WaitGroup
	results := make(chan []nav.Result, runtime.NumCPU())
	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range results {
			for _, res := range result {
				txt := fmt.Sprintf("%q", res.Owner)
				if !res.Valid {
					txt = "INVALID"
				}
				fmt.Fprintf(os.Stdout, "%s;%s\n", res.TaxNo, txt)
			}
		}
	}()

	ep := &nav.Endpoint{URL: *flagURL, BatchSize: *flagBatchSize}
	logger := Log
	var err error
	ctx := context.Background()
	if *flagTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *flagTimeout)
		defer cancel()
	}
	if flag.NArg() > 0 {
		logger = logger.New("name", "Get", "args", flag.Args())
		logger.Info("call")
		var result []nav.Result
		logger.Info("Start")
		result, err = ep.Get(ctx, flag.Args())
		if result != nil {
			results <- result
		}
		close(results)
	} else {
		logger = logger.New("name", "GetFromReader")
		logger.Info("Start")
		err = ep.GetFromReader(ctx, results, os.Stdin)
	}
	wg.Wait()
	if err != nil {
		logger.Error("get", "error", err)
		os.Exit(2)
	}
}
