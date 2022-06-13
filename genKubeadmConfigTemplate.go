package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

func generateKubeadmConfigTemplate(cluster Cluster) {
	for nodesetName := range cluster.NodePools {

		kctStr1 := "#!/bin/bash\n" +
			"for i in $(ls /run/kubeadm/ | grep 'kubeadm.yaml\\|kubeadm-join-config.yaml'); do\n" +
			"  cat <<EOF>> \"/run/kubeadm//$i\"\n" +
			"---\n" +
			"kind: KubeProxyConfiguration\n" +
			"apiVersion: kubeproxy.config.k8s.io/v1alpha1\n" +
			"metricsBindAddress: \"0.0.0.0:10249\"\n" +
			"EOF\n" +
			"done"

		kctStr2 := "[metrics]\n" +
			"  address = \"0.0.0.0:1338\"\n" +
			"  grpc_histogram = false"

		kct := KubeadmConfigTemplate{}
		kct.APIVersion = "bootstrap.cluster.x-k8s.io/v1beta1"
		kct.Kind = "KubeadmConfigTemplate"
		kct.Metadata.Name = cluster.MetaData.Name + "-" + nodesetName
		kct.Metadata.Namespace = "default"
		kct.Spec.Template.Spec.Files = append(kct.Spec.Template.Spec.Files, struct {
			Content     string "yaml:\"content\""
			Path        string "yaml:\"path\""
			Permissions string "yaml:\"permissions\""
		}{
			Content:     kctStr1,
			Path:        "/run/kubeadm/konvoy-set-kube-proxy-configuration.sh",
			Permissions: "0700",
		})
		kct.Spec.Template.Spec.Files = append(kct.Spec.Template.Spec.Files, struct {
			Content     string "yaml:\"content\""
			Path        string "yaml:\"path\""
			Permissions string "yaml:\"permissions\""
		}{
			Content:     kctStr2,
			Path:        "/etc/containerd/conf.d/konvoy-metrics.toml",
			Permissions: "0644",
		})
		kct.Spec.Template.Spec.Format = "cloud-config"
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.CriSocket = "/run/containerd/containerd.sock"
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.CloudProvider = ""
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.VolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
		kct.Spec.Template.Spec.PreKubeadmCommands = append(kct.Spec.Template.Spec.PreKubeadmCommands,
			"systemctl daemon-reload",
			"systemctl restart containerd",
			"/run/kubeadm/konvoy-set-kube-proxy-configuration.sh")

		if cluster.NodePools[nodesetName].Flags["gpu"] {
			kct.Spec.Template.Spec.PostKubeadmCommands = append(kct.Spec.Template.Spec.PostKubeadmCommands,
				"sudo shutdown -r 5 & exit 0")
		}
		data, err := yaml.Marshal(&kct)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-KubeadmConfigTemplate.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
}
