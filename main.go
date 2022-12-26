package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/dynamic"
	// "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Prepare kubeconfig
	var kubeConfigName string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("error getting user home dir: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Enter kubeconfig file to use from %s :", filepath.Join(userHomeDir, ".kube"))
	fmt.Scanln(&kubeConfigName)

	kubeConfigPath := filepath.Join(userHomeDir, ".kube", kubeConfigName)
	fmt.Printf("Using kubeconfig: %s\n", kubeConfigPath)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting Kubernetes config: %v\n", err)
		os.Exit(1)
	}

	// Create dynamic client for k8s
	ingressRouteClient, err := newClient(kubeConfig)
	if err != nil {
		fmt.Printf("Cannot create dynamic interface: %v\n", err)
		os.Exit(1)
	}

	// Get the list of IngressRoutes
	ctx := context.Background()
	ingressRouteList, err := ListIngressRoutes(ctx, ingressRouteClient, "")
	if err != nil {
		fmt.Printf("Cannot create ingressroute list: %v\n", err)
		os.Exit(1)
	}

	// Start processing the list
	hostNames := make(map[string]string, len(ingressRouteList))
	for _, ingressRoute := range ingressRouteList {
		// Convert the list
		unstructured := ingressRoute.UnstructuredContent()
		for _, v := range unstructured {
			// Use regex to get the DNS names
			temp := fmt.Sprint(v)
			re := regexp.MustCompile("`(.*?)`")
			match := re.FindStringSubmatch(temp)
			if len(match) > 0 {
				// remove first and last characters
				match[0] = match[0][1 : len(match[0])-1]
				// resolve dns
				ip, err := resolveDNS(match[0])
				if err != nil {
					continue
				}
				// save DNS name and IP
				hostNames[match[0]] = ip
			}
		}
	}

	// Create csv file
	csvFile, err := os.Create("IngressRoutes-DNS-IP.csv")
	if err != nil {
		// log.Fatalf("failed creating file: %s", err)
		fmt.Printf("Failed to create csv file: %s", err)
		os.Exit(1)
	}

	// Write to the csv file
	csvwriter := csv.NewWriter(csvFile)
	// Write first row
	firstRow := make([]string, 0)
	firstRow = append(firstRow, "name")
	firstRow = append(firstRow, "ip")
	err = csvwriter.Write(firstRow)
	if err != nil {
		fmt.Printf("Failed to write in csv file: %s", err)
		os.Exit(1)
	}

	// Write all hosts and ips
	for host, ip := range hostNames {
		r := make([]string, 0)
		r = append(r, host)
		r = append(r, ip)
		err := csvwriter.Write(r)
		if err != nil {
			fmt.Printf("Failed to write in csv file: %s", err)
			os.Exit(1)
		}
	}

	csvwriter.Flush()
	csvFile.Close()
}

func newClient(config *rest.Config) (dynamic.Interface, error) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return dynClient, nil
}

func ListIngressRoutes(ctx context.Context, client dynamic.Interface, namespace string) ([]unstructured.Unstructured, error) {
	// point schema to use
	var ingressRouteResource = schema.GroupVersionResource{Group: "traefik.containo.us", Version: "v1alpha1", Resource: "ingressroutes"}
	// GET /apis/traefik.containo.us/v1alpha1/namespaces/{namespace}/ingressroutes/
	list, err := client.Resource(ingressRouteResource).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func resolveDNS(name string) (string, error) {
	ips, err := net.LookupIP(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get IPs: %v\n", err)
		return "", err
	}
	return ips[0].String(), err
}
