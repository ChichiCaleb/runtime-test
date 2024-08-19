package controller

import (
    "context"
    "fmt"
    "path/filepath"
    "sync"            
    "os"              
    "encoding/json"   
    "net/http"        
    "time"            
    "bytes"          
    "io" 
    "gopkg.in/yaml.v2"             

    apiclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
    applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
    applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
    appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
    
    "go.uber.org/zap"
    "k8s.io/client-go/kubernetes"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/retry"
    "k8s.io/apimachinery/pkg/api/errors"
   

    "github.com/ChichiCaleb/runtimetest/apis/v1alpha1"
)

var (
    token     string
    tokenLock sync.Mutex
)

type Controller struct {
	Clientset        kubernetes.Interface
	dynClient        dynamic.Interface
	logger           *zap.SugaredLogger

	argoClient   apiclient.Client
	appSetClient   applicationsetpkg.ApplicationSetServiceClient
	appClient    applicationpkg.ApplicationServiceClient
	
}

// NewController initializes a new controller
func NewController(clientset kubernetes.Interface, dynClient dynamic.Interface,  logger *zap.SugaredLogger) *Controller {
	ctrl := &Controller{
		Clientset:       clientset,
		dynClient:       dynClient,
		logger:          logger,
	
	}

   return ctrl
}

// NewInClusterController initializes a new controller for in-cluster execution
func NewInClusterController(logger *zap.SugaredLogger) *Controller {

    // Expand the tilde manually
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Fatalf("Error finding home directory: %v", err)
	}
	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

	// Load kubeconfig and create clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		logger.Fatalf("Error loading kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Error creating Kubernetes clientset: %v", err)
	}

   // Check if ArgoCD is installed and ready
	installed, ready, err := isArgoCDInstalledAndReady(logger, clientset)
	if err != nil {
		logger.Fatalf("Error checking ArgoCD installation status: %v", err)
	}

	if !installed {
		logger.Fatalf("ArgoCD is not installed in the cluster")
	}

	if !ready {
		logger.Fatalf("ArgoCD components are installed but not ready")
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Error creating dynamic Kubernetes client: %v", err)
	}

	return NewController(clientset, dynClient, logger)
}

// CreateArgoCDClient creates and returns an Argo CD client
func CreateArgoCDClient(authToken string) (apiclient.Client, error) {
   
    argoURL := "argo-cd-argocd-server.argocd.svc.cluster.local"

    // Create Argo CD client options with the token
    argoClientOpts := apiclient.ClientOptions{
        ServerAddr: argoURL,
        AuthToken:  authToken,
		PlainText: true,
    }

    // Create the Argo CD client
    argoClient, err := apiclient.NewClient(&argoClientOpts)
    if err != nil {
        return nil, fmt.Errorf("failed to create Argo CD client: %v", err)
    }

    return argoClient, nil
}

func refreshClients(c *Controller, newToken string) error {
    // Update the Argo CD client with the new token
    newArgoClient, err := CreateArgoCDClient(newToken)
    if err != nil {
        return fmt.Errorf("failed to create Argo CD client: %v", err)
    }

    // Update the clients stored in the controller struct
    c.argoClient = newArgoClient

   

   appsetconn, newAppSetClient, err := newArgoClient.NewApplicationSetClient()
    if err != nil {
       
        return fmt.Errorf("failed to create ApplicationSet client: %v", err)
    }

	_, newappClient, err := newArgoClient.NewApplicationClient()
	if err != nil {
		appsetconn.Close()
		return fmt.Errorf("failed to create Application client: %v", err)
	}
	

    c.appSetClient = newAppSetClient
	c.appClient = newappClient


    return nil
}

func getAdminPassword(clientset kubernetes.Interface) (string, error) {
    namespace := "argocd"
    secretName := "argocd-initial-admin-secret"

    // Retrieve the secret from Kubernetes
    secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to get secret: %v", err)
    }

    // Ensure the password field exists
    passwordBytes, exists := secret.Data["password"]
    if !exists {
        return "", fmt.Errorf("password not found in secret")
    }

    // Print the raw password for debugging
    password := string(passwordBytes)
   


    return password, nil
}

func GenerateAuthToken(password string) (string, error) {
    argoURL := "http://argo-cd-argocd-server.argocd.svc.cluster.local/api/v1/session"
    payload := map[string]string{
        "username": "admin",
        "password": password,
    }
    payloadBytes, _ := json.Marshal(payload)


    client := &http.Client{
        Timeout: 30 * time.Second,
    }

    req, err := http.NewRequest("POST", argoURL, bytes.NewBuffer(payloadBytes))
    if err != nil {
        return "", fmt.Errorf("failed to create request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to send request: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := io.ReadAll(resp.Body)
      return "", fmt.Errorf("authentication failed: %v, response: %s", resp.Status, string(bodyBytes))
    }

    var response map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return "", fmt.Errorf("failed to decode response: %v", err)
    }

    token, ok := response["token"].(string)
    if !ok {
       
      return "", fmt.Errorf("token not found in response")
    }


    return token, nil
}

// GetAuthToken retrieves the current token, generating a new one if necessary
func GetAuthToken(c *Controller, password string) (string, error) {
    tokenLock.Lock()
    defer tokenLock.Unlock()

    if token == "" {
        var err error
        token, err = GenerateAuthToken(password)
        if err != nil {
            return "", fmt.Errorf("failed to generate auth token: %v", err)
        }
        go scheduleTokenRefresh(c, password)
    }

    return token, nil
}


// scheduleTokenRefresh sets up a routine to refresh the token before it expires and update clients
func scheduleTokenRefresh(c *Controller, password string) {
    for {
        // Schedule the token refresh before the actual expiry time
        time.Sleep(23 * time.Hour) // Assuming a 24-hour token expiry, adjust as necessary
        tokenLock.Lock()

        newToken, err := GenerateAuthToken(password)
        if err != nil {
            fmt.Printf("failed to refresh token: %v\n", err)
            tokenLock.Unlock()
            continue
        }

        token = newToken
        err = refreshClients(c, newToken) // Pass controller reference
        if err != nil {
            fmt.Printf("failed to refresh clients: %v\n", err)
        }

        tokenLock.Unlock()
    }
}


// UpdateStatus updates the status of the custom CRD using the provided dynamic client and logger
func UpdateStatus(logger *zap.SugaredLogger, dynamicClient dynamic.Interface, observed *v1alpha1.App, status v1alpha1.AppStatus) error {
    err := updateStatus(logger, dynamicClient, observed.ObjectMeta.Name, observed.ObjectMeta.Namespace, status)

    if err != nil {
        logger.Errorf("Failed to update status for %s/%s: %v", observed.ObjectMeta.Namespace, observed.ObjectMeta.Name, err)
        return err
    }
    return nil
}

// updateStatus is the actual implementation that performs the update operation
func updateStatus(logger *zap.SugaredLogger, dynamicClient dynamic.Interface, name, namespace string, status v1alpha1.AppStatus) error {
    defer func() {
        if r := recover(); r != nil {
            logger.Errorf("Recovered from panic while updating status for %s/%s: %v", namespace, name, r)
        }
    }()

    gvr := schema.GroupVersionResource{
        Group:    "alustan.io",
        Version:  "v1alpha1",
        Resource: "apps",
    }

    retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
        unstructuredApp, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
        if err != nil {
            if errors.IsNotFound(err) {
                logger.Infof("Resource %s in namespace %s does not exist, assuming it has been deleted", name, namespace)
                return nil
            }
            return err
        }

        app := &v1alpha1.App{}
        err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredApp.Object, app)
        if err != nil {
            return fmt.Errorf("failed to convert unstructured data to App: %v", err)
        }

        app.Status = status

        updatedUnstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app)
        if err != nil {
            return fmt.Errorf("failed to convert App to unstructured data: %v", err)
        }
        updatedUnstructured := &unstructured.Unstructured{Object: updatedUnstructuredMap}

        _, err = dynamicClient.Resource(gvr).Namespace(namespace).UpdateStatus(context.Background(), updatedUnstructured, metav1.UpdateOptions{})
        if err != nil {
            return err
        }

        return nil
    })

    if retryErr != nil {
        logger.Errorf("Failed to update status for resource %s in namespace %s after retrying: %v", name, namespace, retryErr)
        return retryErr
    }

    logger.Infof("Updated status for %s in namespace %s", name, namespace)
    return nil
}


func (c *Controller) RunController() {
    // Authenticate and create ArgoCD client
    password, err := getAdminPassword(c.Clientset)
    if err != nil {
        c.logger.Fatalf("Failed to get admin password: %v", err)
    }

    authToken, err := GetAuthToken(c, password)
    if err != nil {
        c.logger.Fatalf("Failed to get auth token: %v", err)
    }

    err = refreshClients(c, authToken)
    if err != nil {
        c.logger.Fatalf("Failed to refresh clients: %v", err)
    }

    c.logger.Info("Successfully created ArgoCD client")
    c.logger.Infof("Successfully created ApplicationSet client")
    c.logger.Infof("Successfully created Application client")
    c.logger.Info("App controller successfully instantiated!!!")

    // Create an ApplicationSet (example YAML definition)
    appSetYAML := `
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: example-appset
  namespace: argocd
spec:
  generators:
    - list:
        elements:
          - cluster: "my-cluster"
            url: "https://kubernetes.default.svc"
  template:
    metadata:
      name: "{{cluster}}-app"
    spec:
      project: default
      source:
        repoURL: "https://github.com/argoproj/argocd-example-apps"
        targetRevision: HEAD
        path: guestbook
      destination:
        server: "{{url}}"
        namespace: guestbook
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
`

    c.logger.Info("Creating ApplicationSet in ArgoCD.")

    var appSet appv1alpha1.ApplicationSet
    err = yaml.Unmarshal([]byte(appSetYAML), &appSet)
    if err != nil {
        c.logger.Fatalf("Failed to unmarshal ApplicationSet YAML: %v", err)
    }

    err = retry.OnError(retry.DefaultRetry, errors.IsInternalError, func() error {
        _, err = c.appSetClient.Create(context.Background(), &applicationsetpkg.ApplicationSetCreateRequest{
            Applicationset: &appSet,
        })
        return err
    })

    if err != nil {
        c.logger.Fatalf("Failed to create ApplicationSet: %v", err)
    }

    c.logger.Infof("Successfully applied ApplicationSet '%s' using ArgoCD", appSet.Name)

    // Wait for a short period to allow the ApplicationSet to be processed
    time.Sleep(15 * time.Second)

    name := "my-cluster-app"
    argocdNamespace := "argocd"

    app, err := c.appClient.Get(context.Background(), &applicationpkg.ApplicationQuery{
        Name:         &name,
        AppNamespace: &argocdNamespace,
    })
    if err != nil {
        c.logger.Fatalf("Failed to get application: %v", err)
    }

    // Populate the AppStatus structure
    appStatus := v1alpha1.AppStatus{  // Use the correct reference to AppStatus
        HealthStatus: app.Status,
    }

    // Update the status using the UpdateStatus function
    observed := &v1alpha1.App{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "example-app",
            Namespace: "default",
        },
        Status: appStatus, // Assign the populated AppStatus
    }

    err = UpdateStatus(c.logger, c.dynClient, observed, appStatus)
    if err != nil {
        c.logger.Fatalf("Failed to update status: %v", err)
    }

    c.logger.Infof("Status updated successfully")
}




func isArgoCDInstalledAndReady(logger *zap.SugaredLogger, clientset kubernetes.Interface) (bool, bool, error) {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "argocd", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}

	// Check for the presence and readiness of ArgoCD components
	deployments := []string{
		"argo-cd-argocd-applicationset-controller",
		"argo-cd-argocd-notifications-controller",
		"argo-cd-argocd-server",
		"argo-cd-argocd-repo-server",
		"argo-cd-argocd-redis",
		"argo-cd-argocd-dex-server",
	}

	for _, deployment := range deployments {
		deploy, err := clientset.AppsV1().Deployments("argocd").Get(context.TODO(), deployment, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ArgoCD Components not found. Installing...")
				return false, false, nil
			}
			return false, false, err
		}

		// Check if the number of ready replicas matches the desired replicas
		if deploy.Status.ReadyReplicas != *deploy.Spec.Replicas {
			return true, false, nil // Components are installed but not ready
		}
	}

	return true, true, nil // All components are installed and ready
}