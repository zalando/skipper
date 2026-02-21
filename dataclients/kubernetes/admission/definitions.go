package admission

import "encoding/json"

type groupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

type groupVersionResource struct {
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
}

type typeMeta struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

type objectMeta struct {
	Name         string `json:"name,omitempty"`
	GenerateName string `json:"generateName,omitempty"`
}

type partialObjectMetadata struct {
	typeMeta   `json:",inline"`
	objectMeta `json:"metadata"`
}

type admissionReview struct {
	typeMeta `json:",inline"`
	Request  *admissionRequest  `json:"request,omitempty"`
	Response *admissionResponse `json:"response,omitempty"`
}

type admissionRequest struct {
	UID         string               `json:"uid"`
	Kind        groupVersionKind     `json:"kind"`
	Resource    groupVersionResource `json:"resource"`
	SubResource string               `json:"subResource,omitempty"`
	Name        string               `json:"name,omitempty"`
	Namespace   string               `json:"namespace,omitempty"`
	Operation   string               `json:"operation"`
	UserInfo    userInfo             `json:"userInfo"`
	Object      json.RawMessage      `json:"object,omitempty"`
}

type admissionResponse struct {
	UID     string  `json:"uid"`
	Allowed bool    `json:"allowed"`
	Result  *status `json:"status,omitempty"`
}

type status struct {
	Message string `json:"message,omitempty"`
}

type userInfo struct {
	Username string `json:"username,omitempty"`
}
