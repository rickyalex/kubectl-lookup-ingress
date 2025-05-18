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

	if flag.NArg() < 2 {
		fmt.Println("Usage: kubectl lookup-ingress <deployment|service> <name> [-n namespace]")
		os.Exit(1)
	}

	kind := strings.ToLower(flag.Arg(0))
	name := flag.Arg(1)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}
	fmt.Println("Using kubeconfig:", kubeconfig)

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

	fmt.Printf("\n%-30s | %-25s | %-20s | %-20s\n", "Ingress Name", "Host", "Path", "Service Name")
	fmt.Println(strings.Repeat("-", 105))

	for _, entry := range entries {
		fmt.Printf("%-30s | %-25s | %-20s | %-20s\n",
			entry["ingress"], entry["host"], entry["path"], entry["service"])
	}
	fmt.Println()
}