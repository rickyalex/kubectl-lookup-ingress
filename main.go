package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	var namespace string
	flag.StringVar(&namespace, "n", "default", "Namespace of the resource")
	flag.Parse()

	fmt.Printf("Using namespace: %s\n", namespace)

	if flag.NArg() < 2 {
		fmt.Println("Usage: kubectl lookupingress [-n namespace] <deployment|service> <name>")
		os.Exit(1)
	}

	kind := strings.ToLower(flag.Arg(0))
	name := flag.Arg(1)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	ingressList, err := clientset.NetworkingV1().Ingresses(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing ingresses: %v\n", err)
		os.Exit(1)
	}

	var results []map[string]string

	for _, ingress := range ingressList.Items {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				backend := path.Backend.Service
				if backend == nil {
					continue
				}

				if kind == "service" && backend.Name == name {
					results = append(results, map[string]string{
						"ingress": ingress.Name,
						"host":    rule.Host,
						"path":    path.Path,
						"service": backend.Name,
					})
				}
			}
		}
	}

	if kind == "deployment" {
		svcList, err := clientset.CoreV1().Services(namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing services: %v\n", err)
			os.Exit(1)
		}

		for _, svc := range svcList.Items {
			if svc.Spec.Selector == nil {
				continue
			}

			dep, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				continue
			}

			match := true
			for key, val := range svc.Spec.Selector {
				if depVal, ok := dep.Spec.Selector.MatchLabels[key]; !ok || depVal != val {
					match = false
					break
				}
			}

			if match {
				for _, ingress := range ingressList.Items {
					for _, rule := range ingress.Spec.Rules {
						for _, path := range rule.HTTP.Paths {
							backend := path.Backend.Service
							if backend == nil {
								continue
							}

							if backend.Name == svc.Name {
								results = append(results, map[string]string{
									"ingress": ingress.Name,
									"host":    rule.Host,
									"path":    path.Path,
									"service": svc.Name,
								})
							}
						}
					}
				}
			}
		}
	}

	printIngressTable(results)
}

func printIngressTable(entries []map[string]string) {
	if len(entries) == 0 {
		fmt.Println("No associated ingress found.")
		return
	}

	// Define headers
	headers := []string{"Ingress Name", "Host", "Path", "Service Name"}
	keys := []string{"ingress", "host", "path", "service"} // Corresponding keys in the map

	// Initialize column widths with the length of the headers themselves
	colWidths := make(map[string]int)
	for i, header := range headers {
		colWidths[keys[i]] = len(header)
	}

	// First pass: Iterate through entries to find the maximum width for each column
	for _, entry := range entries {
		for _, key := range keys {
			colWidths[key] = max(colWidths[key], len(entry[key]))
		}
	}

	// Construct the format string dynamically based on calculated widths
	// Add some padding (e.g., 2 spaces) for better readability between columns
	padding := 2
	headerFormat := ""
	rowFormat := ""
	totalWidth := 0

	for i, key := range keys {
		width := colWidths[key]
		headerFormat += fmt.Sprintf("%%-%ds", width) // Left-align header
		rowFormat += fmt.Sprintf("%%-%ds", width)    // Left-align data

		totalWidth += width
		if i < len(keys)-1 {
			separator := strings.Repeat(" ", padding) + "|" + strings.Repeat(" ", padding)
			headerFormat += separator
			rowFormat += separator
			totalWidth += len(separator)
		}
	}
	headerFormat += "\n"
	rowFormat += "\n"

	// Print the header
	headerValues := make([]interface{}, len(headers))
	for i, h := range headers {
		headerValues[i] = h
	}
	fmt.Printf(headerFormat, headerValues...)

	// Print the separator line
	fmt.Println(strings.Repeat("-", totalWidth))

	// Second pass: Print the data rows
	for _, entry := range entries {
		rowValues := make([]interface{}, len(keys))
		for i, key := range keys {
			rowValues[i] = entry[key]
		}
		fmt.Printf(rowFormat, rowValues...)
	}
	fmt.Println()
}