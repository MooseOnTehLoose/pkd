package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func generateKubeadmConfigTemplate(cluster pkdCluster) {
	for nodesetName := range cluster.NodePools {

		//konvoy-set-kube-proxy-configuration.sh
		kctStr1 := "#!/bin/bash\n" +
			"for i in $(ls /run/kubeadm/ | grep 'kubeadm.yaml\\|kubeadm-join-config.yaml'); do\n" +
			"  cat <<EOF>> \"/run/kubeadm//$i\"\n" +
			"---\n" +
			"kind: KubeProxyConfiguration\n" +
			"apiVersion: kubeproxy.config.k8s.io/v1alpha1\n" +
			"metricsBindAddress: \"0.0.0.0:10249\"\n" +
			"EOF\n" +
			"done"

		//metrics-toml (not a prekubeadmcommand)
		kctStr2 := "[metrics]\n" +
			"  address = \"0.0.0.0:1338\"\n" +
			"  grpc_histogram = false"

		//containerd-apply-patches.sh
		kctStr3 := "#!/bin/bash\n" +
			"set -euo pipefail\n" +
			"IFS=$'\\n\\t'\n" +
			"declare -r TOML_MERGE_IMAGE='ghcr.io/mesosphere/toml-merge:v0.2.0'\n" +
			"if ! ctr --namespace k8s.io images check \"name==${TOML_MERGE_IMAGE}\" | grep \"${TOML_MERGE_IMAGE}\" >/dev/null; then\n" +
			"  ctr --namespace k8s.io images pull \"${TOML_MERGE_IMAGE}\"\n" +
			"fi\n" +
			"cleanup() {\n" +
			"  ctr images unmount \"${tmp_ctr_mount_dir}\" || true\n" +
			"}\n" +
			"trap 'cleanup' EXIT\n" +
			"readonly tmp_ctr_mount_dir=\"$(mktemp -d)\"\n" +
			"ctr --namespace k8s.io images mount \"${TOML_MERGE_IMAGE}\" \"${tmp_ctr_mount_dir}\"\n" +
			"\"${tmp_ctr_mount_dir}/usr/local/bin/toml-merge\" -i --patch-file '/etc/containerd/konvoy-conf.d/*.toml' /etc/containerd/config.toml\n"

		//restart-containerd-and-wait.sh
		kctStr4 := "#!/bin/bash\nsystemctl restart containerd\n\nSECONDS=0\nuntil crictl info\ndo\n  if (( SECONDS > 60 ))\n  then\n     echo \"Containerd is not running. Giving up...\"\n     exit 1\n  fi\n  echo \"Containerd is not running yet. Waiting...\"\n  sleep 5\ndone"

		//install-kubelet-credential-providers.sh
		kctStr5 := "#!/bin/bash\n" +
			"set -euo pipefail\n" +
			"IFS=$'\\n\\t'\n" +
			"declare -r CREDENTIAL_PROVIDER_IMAGE='ghcr.io/mesosphere/dynamic-credential-provider:v0.2.0'\n" +
			"if ! ctr --namespace k8s.io images check \"name==${CREDENTIAL_PROVIDER_IMAGE}\" | grep \"${CREDENTIAL_PROVIDER_IMAGE}\" >/dev/null; then\n" +
			"  ctr --namespace k8s.io images pull \"${CREDENTIAL_PROVIDER_IMAGE}\"\n" +
			"fi\n" +
			"cleanup() {\n" +
			"  ctr images unmount \"${tmp_ctr_mount_dir}\" || true\n" +
			"}\n" +
			"trap 'cleanup' EXIT\n" +
			"readonly tmp_ctr_mount_dir=\"$(mktemp -d)\"\n" +
			"export CREDENTIAL_PROVIDER_SOURCE_DIR=\"${tmp_ctr_mount_dir}/opt/image-credential-provider/bin/\"\n" +
			"export CREDENTIAL_PROVIDER_TARGET_DIR=\"/etc/kubernetes/image-credential-provider/\"\n" +
			"ctr --namespace k8s.io images mount \"${CREDENTIAL_PROVIDER_IMAGE}\" \"${tmp_ctr_mount_dir}\"\n" +
			"\"${tmp_ctr_mount_dir}/opt/image-credential-provider/bin/dynamic-credential-provider\" install"

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
		kct.Spec.Template.Spec.Files = append(kct.Spec.Template.Spec.Files, struct {
			Content     string "yaml:\"content\""
			Path        string "yaml:\"path\""
			Permissions string "yaml:\"permissions\""
		}{
			Content:     kctStr3,
			Path:        "/run/konvoy/containerd-apply-patches.sh",
			Permissions: "0700",
		})
		kct.Spec.Template.Spec.Files = append(kct.Spec.Template.Spec.Files, struct {
			Content     string "yaml:\"content\""
			Path        string "yaml:\"path\""
			Permissions string "yaml:\"permissions\""
		}{
			Content:     kctStr4,
			Path:        "/run/konvoy/restart-containerd-and-wait.sh",
			Permissions: "0700",
		})
		kct.Spec.Template.Spec.Files = append(kct.Spec.Template.Spec.Files, struct {
			Content     string "yaml:\"content\""
			Path        string "yaml:\"path\""
			Permissions string "yaml:\"permissions\""
		}{
			Content:     kctStr5,
			Path:        "/run/konvoy/install-kubelet-credential-providers.sh",
			Permissions: "0700",
		})
		kct.Spec.Template.Spec.Format = "cloud-config"
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.CriSocket = "/run/containerd/containerd.sock"
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.CloudProvider = ""
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.ProviderID = "'{{ .ProviderID }}'"
		kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.VolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
		kct.Spec.Template.Spec.PreKubeadmCommands = append(kct.Spec.Template.Spec.PreKubeadmCommands,
			"/run/kubeadm/konvoy-set-kube-proxy-configuration.sh",
			"/run/konvoy/install-kubelet-credential-providers.sh",
			"/run/konvoy/containerd-apply-patches.sh",
			"systemctl daemon-reload",
			"/run/konvoy/restart-containerd-and-wait.sh")

		//you must restart gpu nodes after deploying! gpu drivers wont function until after restart
		if cluster.NodePools[nodesetName].Flags["gpu"] {
			kct.Spec.Template.Spec.PostKubeadmCommands = append(kct.Spec.Template.Spec.PostKubeadmCommands,
				"sudo shutdown -r 5 & exit 0")
		}
		data, err := yaml.Marshal(&kct)
		if err != nil {
			log.Fatal(err)
		}
		err = os.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-KubeadmConfigTemplate.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
}
