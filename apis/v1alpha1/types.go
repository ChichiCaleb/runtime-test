package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	 "k8s.io/apimachinery/pkg/runtime"
    appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// +groupName=test.io

// AppSpec defines the desired state of App
type AppSpec struct {
	AppName      string `json:"appName,omitempty"`
	AppNamespace string `json:"appNamespace,omitempty"`
}

// AppStatus defines the observed state of App
type AppStatus struct {
	HealthStatus   appv1alpha1.ApplicationStatus   `json:"healthStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=apps,scope=Namespaced

// App is the Schema for the apps API
type App struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   AppSpec   `json:"spec,omitempty"`
    Status AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppList contains a list of App
type AppList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []App `json:"items"`
}



