module github.com/ChichiCaleb/runtimetest

go 1.22.2

require (
    k8s.io/api v0.26.11
    k8s.io/apimachinery v0.26.11
    k8s.io/client-go v0.26.11
)

replace k8s.io/kubernetes => k8s.io/kubernetes v1.26.11

