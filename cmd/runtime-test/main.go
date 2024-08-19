package main

import (
	"fmt"
	"os"
	

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/ChichiCaleb/runtimetest/controller"
	"github.com/ChichiCaleb/runtimetest/apis/v1alpha1"
	"go.uber.org/zap"

)


func init() {
	// Register the custom resource types with the global scheme
	utilruntime.Must(v1alpha1.AddToScheme(runtime.NewScheme()))
}


func main() {
    // Set up logging
     // Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	

	defer logger.Sync() // Ensure logger is flushed on shutdown
	sugar := logger.Sugar()

    // Create a controller and pass the logger
	ctrl := controller.NewInClusterController(sugar)

    // Run controller
	ctrl.RunController()
}


