/*
Copyright 2018 The Kubernetes Authors.

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
	"net/http"
)

var (
	certsDir = flag.String("certs-dir", "/etc/tls-certs", `Where the TLS cert files are stored.`)
)

func main() {
	flag.Parse()
	certs := initCerts(*certsDir)
	as := &admissionServer{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		as.serve(w, r)
	})
	clientset := getClient()
	server := &http.Server{
		Addr:      ":8000",
		TLSConfig: configTLS(clientset, certs.serverCert, certs.serverKey),
	}
	go selfRegistration(clientset, certs.caCert)
	server.ListenAndServeTLS("", "")
}
