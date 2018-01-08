package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dlorenc/kbuild/pkg/dockerfile"
	"github.com/dlorenc/kbuild/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"

	batch "k8s.io/client-go/pkg/apis/batch/v1"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	var dockerfilePath = flag.String("dockerfile", "./Dockerfile", "path to dockerfile")
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	w := v1.VolumeMount{
		Name:      "workdir",
		MountPath: "/work-dir",
	}

	b, err := ioutil.ReadFile(*dockerfilePath)
	if err != nil {
		log.Fatal(err)
	}

	stages, err := dockerfile.Parse(b)
	from := stages[0].BaseName

	cfg := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "build-dockerfile",
		},
		Data: map[string]string{
			"Dockerfile": string(b),
		},
	}

	cfgmap, err := clientset.CoreV1().ConfigMaps("default").Create(cfg)
	if err != nil {
		log.Fatal(err)
	}

	j := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "build-job-",
		},
		Spec: batch.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{
							Name:  "init-static",
							Image: "gcr.io/dlorenc-vmtest2/kbuild-static:latest",
							Command: []string{
								"cp", "/bin/main", "/work-dir/",
							},
							VolumeMounts: []v1.VolumeMount{w},
						},
						{
							Name:         "do-build",
							Image:        from,
							Command:      []string{"/work-dir/main"},
							VolumeMounts: []v1.VolumeMount{w, v1.VolumeMount{Name: "dockerfile", MountPath: "/dockerfile"}},
						},
					},
					Containers: []v1.Container{
						{
							Name:         "append",
							Image:        "gcr.io/dlorenc-vmtest2/appender:latest",
							Command:      []string{"python", "/app/main.py"},
							VolumeMounts: []v1.VolumeMount{w},
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{
							Name: "workdir",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "dockerfile",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: cfgmap.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	job, err := clientset.BatchV1().Jobs("default").Create(j)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created build job: ", job.Name)

	stopCh := make(chan bool)

	for {
		j, err = clientset.BatchV1().Jobs("default").Get(job.Name, metav1.GetOptions{})
		if err != nil {
			// wait until the job exists
			continue
		}
		break
	}

	// Stream logs
	for {
		opts := metav1.ListOptions{LabelSelector: labels.Set(j.Spec.Selector.MatchLabels).AsSelector().String()}
		jobPods, err := clientset.CoreV1().Pods("default").List(opts)
		if err != nil {
			continue
		}
		// Stream logs
		for _, p := range jobPods.Items {
			f := func() {
				streamLogs(clientset, "do-build", p.Name, "default")
			}
			go util.Until(f, stopCh)
		}
		break
	}

	for {
		j, err := clientset.BatchV1().Jobs("default").Get(job.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		if j.Status.CompletionTime == nil {
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("Job finished.")
			stopCh <- true
			break
		}
	}
}

func streamLogs(clientset *kubernetes.Clientset, container, pod, namespace string) error {
	r, err := clientset.CoreV1().Pods(namespace).GetLogs(pod, &v1.PodLogOptions{Container: container, Follow: true}).Stream()
	if err != nil {
		return err
	}
	defer r.Close()
	if _, err := io.Copy(os.Stderr, r); err != nil {
		return err
	}
	return nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
