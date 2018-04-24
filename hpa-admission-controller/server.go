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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type admissionServer struct{}

type patchRecord struct {
	Op    string      `json:"op,inline"`
	Path  string      `json:"path,inline"`
	Value interface{} `json:"value"`
}

func (s *admissionServer) getPatchesForHpaResourceRequest(raw []byte, namespace string) ([]patchRecord, error) {
	hpa := autoscaling.HorizontalPodAutoscaler{}
	if err := json.Unmarshal(raw, &hpa); err != nil {
		return nil, err
	}
	glog.Infof("Admitting hpa %v", hpa.ObjectMeta)
	patches := []patchRecord{}
	for i, metric := range hpa.Spec.Metrics {
		if metric.Type == autoscaling.ExternalMetricSourceType && metric.External != nil {
			name := metric.External.MetricName
			glog.Errorf("External metric %v %v", i, metric.External.MetricName)
			if strings.Contains(name, "/") {
				glog.Errorf("Replacing")
				patches = append(patches, patchRecord{
					Op:    "add",
					Path:  fmt.Sprintf("/spec/metrics/%d/external/metricName", i),
					Value: strings.Replace(name, "/", "\\|", -1)})
			}
		}
	}
	return patches, nil
}

func (s *admissionServer) admit(data []byte) *v1beta1.AdmissionResponse {
	glog.Infof("Got request")
	ar := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(data, &ar); err != nil {
		glog.Error(err)
		return nil
	}
	// The externalAdmissionHookConfiguration registered via selfRegistration
	// asks the kube-apiserver to only send admission request regarding HPAs.
	hpaResource := metav1.GroupVersionResource{Group: "autoscaling", Version: "v2beta1", Resource: "horizontalpodautoscalers"}
	var patches []patchRecord
	var err error

	switch ar.Request.Resource {
	case hpaResource:
		patches, err = s.getPatchesForHpaResourceRequest(ar.Request.Object.Raw, ar.Request.Namespace)
	default:
		patches, err = nil, fmt.Errorf("expected the resource to be %v", hpaResource)
	}

	if err != nil {
		glog.Error(err)
		return nil
	}
	response := v1beta1.AdmissionResponse{}
	response.Allowed = true
	if len(patches) > 0 {
		patch, err := json.Marshal(patches)
		if err != nil {
			glog.Errorf("Cannot marshal the patch %v: %v", patches, err)
			return nil
		}
		patchType := v1beta1.PatchTypeJSONPatch
		response.PatchType = &patchType
		response.Patch = patch
		glog.V(4).Infof("Sending patches: %v", patches)
	}
	return &response
}

func (s *admissionServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	reviewResponse := s.admit(body)
	ar := v1beta1.AdmissionReview{
		Response: reviewResponse,
	}

	resp, err := json.Marshal(ar)
	if err != nil {
		glog.Error(err)
	}
	if _, err := w.Write(resp); err != nil {
		glog.Error(err)
	}
}
