package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

const problemTypeNamespace = "urn:gpu-container-cloud:problem:"

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func writeProblem(response http.ResponseWriter, request *http.Request, problem Problem) {
	if problem.Type == "" {
		code := strings.TrimSpace(problem.Code)
		if code == "" {
			code = "unspecified"
		}
		problem.Type = problemTypeNamespace + code
	}
	if problem.Instance == "" && request != nil {
		problem.Instance = request.URL.Path
	}
	if problem.RequestID == "" && request != nil {
		problem.RequestID = RequestIDFromContext(request.Context())
	}
	response.Header().Set("Content-Type", "application/problem+json")
	response.WriteHeader(problem.Status)
	_ = json.NewEncoder(response).Encode(problem)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
