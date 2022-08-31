package main

import (
	"io/ioutil"
	"log"
	"strconv"

	"gopkg.in/yaml.v3"
)

//if 1 CP use alternate structure, otherwise just generate from defaults
func generateKubeadmControlPlane(cluster pkdCluster) {
	controlPlaneReplicas := strconv.Itoa(len(cluster.Controlplane.Hosts))
	kcp := KubeadmControlPlane{}
	kcp.APIVersion = "controlplane.cluster.x-k8s.io/v1beta1"
	kcp.Kind = "KubeadmControlPlane"
	kcp.Metadata.Name = cluster.MetaData.Name + "-control-plane"
	kcp.Metadata.Namespace = "default"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.AuditLogMaxage = "30"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.AuditLogMaxbackup = "10"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.AuditLogMaxsize = "100"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.AuditLogPath = "/var/log/audit/kube-apiserver-audit.log"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.AuditPolicyFile = "/etc/kubernetes/audit-policy/apiserver-audit-policy.yaml"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.CloudProvider = ""
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs.EncryptionProviderConfig = "/etc/kubernetes/pki/encryption-config.yaml"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraVolumes = append(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraVolumes,
		struct {
			HostPath  string "yaml:\"hostPath\""
			MountPath string "yaml:\"mountPath\""
			Name      string "yaml:\"name\""
		}{
			HostPath:  "/etc/kubernetes/audit-policy/",
			MountPath: "/etc/kubernetes/audit-policy/",
			Name:      "audit-policy",
		})

	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraVolumes = append(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraVolumes,
		struct {
			HostPath  string "yaml:\"hostPath\""
			MountPath string "yaml:\"mountPath\""
			Name      string "yaml:\"name\""
		}{
			HostPath:  "/var/log/kubernetes/audit",
			MountPath: "/var/log/audit/",
			Name:      "audit-logs",
		})

	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager.ExtraArgs.CloudProvider = ""
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager.ExtraArgs.FlexVolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageTag = "3.4.13-0"
	kcp.Spec.KubeadmConfigSpec.Format = "cloud-config"
	kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.CriSocket = "/run/containerd/containerd.sock"
	kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs.CloudProvider = ""
	kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs.ProviderID = "'{{ .ProviderID }}'"
	kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs.VolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
	kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.CriSocket = "/run/containerd/containerd.sock"
	kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.CloudProvider = ""
	kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.ProviderID = "'{{ .ProviderID }}'"
	kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.VolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
	kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands = append(kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands,
		"systemctl daemon-reload")
	kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands = append(kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands,
		"/run/konvoy/restart-containerd-and-wait.sh")
	kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands = append(kcp.Spec.KubeadmConfigSpec.PreKubeadmCommands,
		"/run/kubeadm/konvoy-set-kube-proxy-configuration.sh")
	kcp.Spec.MachineTemplate.InfrastructureRef.APIVersion = "infrastructure.cluster.konvoy.d2iq.io/v1alpha1"
	kcp.Spec.MachineTemplate.InfrastructureRef.Kind = "PreprovisionedMachineTemplate"
	kcp.Spec.MachineTemplate.InfrastructureRef.Name = cluster.MetaData.Name + "-control-plane"
	kcp.Spec.MachineTemplate.InfrastructureRef.Namespace = "default"
	kcp.Spec.Version = "v1.22.8"

	if controlPlaneReplicas == "1" {
		kcp.Spec.Replicas = 1

	} else {
		kcp.Spec.Replicas = len(cluster.Controlplane.Hosts)
		kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge = 0
		kcp.Spec.RolloutStrategy.Type = "RollingUpdate"
	}

	content1 := "# Taken from https://github.com/kubernetes/kubernetes/blob/master/cluster/gce/gci/configure-helper.sh\n# Recommended in Kubernetes docs\napiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n  # The following requests were manually identified as high-volume and low-risk,\n  # so drop them.\n  - level: None\n    users: [\"system:kube-proxy\"]\n    verbs: [\"watch\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"endpoints\", \"services\", \"services/status\"]\n  - level: None\n    # Ingress controller reads 'configmaps/ingress-uid' through the unsecured port.\n    # TODO(#46983): Change this to the ingress controller service account.\n    users: [\"system:unsecured\"]\n    namespaces: [\"kube-system\"]\n    verbs: [\"get\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"configmaps\"]\n  - level: None\n    users: [\"kubelet\"] # legacy kubelet identity\n    verbs: [\"get\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"nodes\", \"nodes/status\"]\n  - level: None\n    userGroups: [\"system:nodes\"]\n    verbs: [\"get\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"nodes\", \"nodes/status\"]\n  - level: None\n    users:\n      - system:kube-controller-manager\n      - system:kube-scheduler\n      - system:serviceaccount:kube-system:endpoint-controller\n    verbs: [\"get\", \"update\"]\n    namespaces: [\"kube-system\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"endpoints\"]\n  - level: None\n    users: [\"system:apiserver\"]\n    verbs: [\"get\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"namespaces\", \"namespaces/status\", \"namespaces/finalize\"]\n  - level: None\n    users: [\"cluster-autoscaler\"]\n    verbs: [\"get\", \"update\"]\n    namespaces: [\"kube-system\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"configmaps\", \"endpoints\"]\n  # Don't log HPA fetching metrics.\n  - level: None\n    users:\n      - system:kube-controller-manager\n    verbs: [\"get\", \"list\"]\n    resources:\n      - group: \"metrics.k8s.io\"\n  # Don't log these read-only URLs.\n  - level: None\n    nonResourceURLs:\n      - /healthz*\n      - /version\n      - /swagger*\n  # Don't log events requests.\n  - level: None\n    resources:\n      - group: \"\" # core\n        resources: [\"events\"]\n  # node and pod status calls from nodes are high-volume and can be large, don't log responses for expected updates from nodes\n  - level: Request\n    users: [\"kubelet\", \"system:node-problem-detector\", \"system:serviceaccount:kube-system:node-problem-detector\"]\n    verbs: [\"update\",\"patch\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"nodes/status\", \"pods/status\"]\n    omitStages:\n      - \"RequestReceived\"\n  - level: Request\n    userGroups: [\"system:nodes\"]\n    verbs: [\"update\",\"patch\"]\n    resources:\n      - group: \"\" # core\n        resources: [\"nodes/status\", \"pods/status\"]\n    omitStages:\n      - \"RequestReceived\"\n  # deletecollection calls can be large, don't log responses for expected namespace deletions\n  - level: Request\n    users: [\"system:serviceaccount:kube-system:namespace-controller\"]\n    verbs: [\"deletecollection\"]\n    omitStages:\n      - \"RequestReceived\"\n  # Secrets, ConfigMaps, and TokenReviews can contain sensitive & binary data,\n  # so only log at the Metadata level.\n  - level: Metadata\n    resources:\n      - group: \"\" # core\n        resources: [\"secrets\", \"configmaps\"]\n      - group: authentication.k8s.io\n        resources: [\"tokenreviews\"]\n    omitStages:\n      - \"RequestReceived\"\n  # Get responses can be large; skip them.\n  - level: Request\n    verbs: [\"get\", \"list\", \"watch\"]\n    resources:\n      - group: \"\" # core\n      - group: \"admissionregistration.k8s.io\"\n      - group: \"apiextensions.k8s.io\"\n      - group: \"apiregistration.k8s.io\"\n      - group: \"apps\"\n      - group: \"authentication.k8s.io\"\n      - group: \"authorization.k8s.io\"\n      - group: \"autoscaling\"\n      - group: \"batch\"\n      - group: \"certificates.k8s.io\"\n      - group: \"extensions\"\n      - group: \"metrics.k8s.io\"\n      - group: \"networking.k8s.io\"\n      - group: \"node.k8s.io\"\n      - group: \"policy\"\n      - group: \"rbac.authorization.k8s.io\"\n      - group: \"scheduling.k8s.io\"\n      - group: \"settings.k8s.io\"\n      - group: \"storage.k8s.io\"\n    omitStages:\n      - \"RequestReceived\"\n  # Default level for known APIs\n  - level: RequestResponse\n    resources:\n      - group: \"\" # core\n      - group: \"admissionregistration.k8s.io\"\n      - group: \"apiextensions.k8s.io\"\n      - group: \"apiregistration.k8s.io\"\n      - group: \"apps\"\n      - group: \"authentication.k8s.io\"\n      - group: \"authorization.k8s.io\"\n      - group: \"autoscaling\"\n      - group: \"batch\"\n      - group: \"certificates.k8s.io\"\n      - group: \"extensions\"\n      - group: \"metrics.k8s.io\"\n      - group: \"networking.k8s.io\"\n      - group: \"node.k8s.io\"\n      - group: \"policy\"\n      - group: \"rbac.authorization.k8s.io\"\n      - group: \"scheduling.k8s.io\"\n      - group: \"settings.k8s.io\"\n      - group: \"storage.k8s.io\"\n    omitStages:\n      - \"RequestReceived\"\n  # Default level for all other requests.\n  - level: Metadata\n    omitStages:\n      - \"RequestReceived\""
	kcp.Spec.KubeadmConfigSpec.Files = append(kcp.Spec.KubeadmConfigSpec.Files, struct {
		Content     string `yaml:"content,omitempty"`
		Path        string `yaml:"path"`
		Permissions string `yaml:"permissions"`
		ContentFrom struct {
			Secret struct {
				Key  string `yaml:"key"`
				Name string `yaml:"name"`
			} `yaml:"secret"`
		} `yaml:"contentFrom,omitempty"`
		Owner string `yaml:"owner,omitempty"`
	}{
		Content:     content1,
		Path:        "/etc/kubernetes/audit-policy/apiserver-audit-policy.yaml",
		Permissions: "0600",
		ContentFrom: struct {
			Secret struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			} "yaml:\"secret\""
		}{},
		Owner: "",
	})

	content2 := "#!/bin/bash\n# CAPI does not expose an API to modify KubeProxyConfiguration\n# this is a workaround to use a script with preKubeadmCommand to modify the kubeadm config files\n# https://github.com/kubernetes-sigs/cluster-api/issues/4512\nfor i in $(ls /run/kubeadm/ | grep 'kubeadm.yaml\\|kubeadm-join-config.yaml'); do\n  cat <<EOF>> \"/run/kubeadm//$i\"\n---\nkind: KubeProxyConfiguration\napiVersion: kubeproxy.config.k8s.io/v1alpha1\nmetricsBindAddress: \"0.0.0.0:10249\"\nEOF\ndone"
	kcp.Spec.KubeadmConfigSpec.Files = append(kcp.Spec.KubeadmConfigSpec.Files, struct {
		Content     string `yaml:"content,omitempty"`
		Path        string `yaml:"path"`
		Permissions string `yaml:"permissions"`
		ContentFrom struct {
			Secret struct {
				Key  string `yaml:"key"`
				Name string `yaml:"name"`
			} `yaml:"secret"`
		} `yaml:"contentFrom,omitempty"`
		Owner string `yaml:"owner,omitempty"`
	}{
		Content:     content2,
		Path:        "/run/kubeadm/konvoy-set-kube-proxy-configuration.sh",
		Permissions: "0700",
		ContentFrom: struct {
			Secret struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			} "yaml:\"secret\""
		}{},
		Owner: "",
	})

	content3 := "[metrics]\n  address = \"0.0.0.0:1338\"\n  grpc_histogram = false"
	kcp.Spec.KubeadmConfigSpec.Files = append(kcp.Spec.KubeadmConfigSpec.Files, struct {
		Content     string `yaml:"content,omitempty"`
		Path        string `yaml:"path"`
		Permissions string `yaml:"permissions"`
		ContentFrom struct {
			Secret struct {
				Key  string `yaml:"key"`
				Name string `yaml:"name"`
			} `yaml:"secret"`
		} `yaml:"contentFrom,omitempty"`
		Owner string `yaml:"owner,omitempty"`
	}{
		Content:     content3,
		Path:        "/etc/containerd/conf.d/konvoy-metrics.toml",
		Permissions: "0644",
		ContentFrom: struct {
			Secret struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			} "yaml:\"secret\""
		}{},
		Owner: "",
	})

	content4 := "#!/bin/bash\nsystemctl restart containerd\n\nSECONDS=0\nuntil crictl info\ndo\n  if (( SECONDS > 60 ))\n  then\n     echo \"Containerd is not running. Giving up...\"\n     exit 1\n  fi\n  echo \"Containerd is not running yet. Waiting...\"\n  sleep 5\ndone"
	kcp.Spec.KubeadmConfigSpec.Files = append(kcp.Spec.KubeadmConfigSpec.Files, struct {
		Content     string `yaml:"content,omitempty"`
		Path        string `yaml:"path"`
		Permissions string `yaml:"permissions"`
		ContentFrom struct {
			Secret struct {
				Key  string `yaml:"key"`
				Name string `yaml:"name"`
			} `yaml:"secret"`
		} `yaml:"contentFrom,omitempty"`
		Owner string `yaml:"owner,omitempty"`
	}{
		Content:     content4,
		Path:        "/run/konvoy/restart-containerd-and-wait.sh",
		Permissions: "0700",
		ContentFrom: struct {
			Secret struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			} "yaml:\"secret\""
		}{},
		Owner: "",
	})
	kcp.Spec.KubeadmConfigSpec.Files = append(kcp.Spec.KubeadmConfigSpec.Files, struct {
		Content     string `yaml:"content,omitempty"`
		Path        string `yaml:"path"`
		Permissions string `yaml:"permissions"`
		ContentFrom struct {
			Secret struct {
				Key  string `yaml:"key"`
				Name string `yaml:"name"`
			} `yaml:"secret"`
		} `yaml:"contentFrom,omitempty"`
		Owner string `yaml:"owner,omitempty"`
	}{
		Content:     "",
		Path:        "/etc/kubernetes/pki/encryption-config.yaml",
		Permissions: "0640",
		ContentFrom: struct {
			Secret struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			} "yaml:\"secret\""
		}{
			Secret: struct {
				Key  string "yaml:\"key\""
				Name string "yaml:\"name\""
			}{
				Key:  "value",
				Name: "cluster-a-etcd-encryption-config",
			},
		},
		Owner: "root:root",
	})

	data, err := yaml.Marshal(&kcp)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-control-plane-KubeadmControlPlane.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

}
