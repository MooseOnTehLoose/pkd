package main

type pkdCluster struct {
	MetaData     MetaData
	AirGap       AirGap
	Registry     Registry
	Controlplane NodePool
	NodePools    map[string]NodePool
}
type NodePool struct {
	Hosts map[string]string
	Flags map[string]bool
}
type AirGap struct {
	Enabled    bool   `yaml:"enabled"`
	OsVersion  string `yaml:"osversion,omitempty"`
	K8sVersion string `yaml:"k8sversion,omitempty"`
}
type MetaData struct {
	DKPversion          string `yaml:"dkpversion"`
	Name                string `yaml:"name"`
	SshUser             string `yaml:"sshuser"`
	SshPrivateKey       string `yaml:"sshprivatekey"`
	InterfaceName       string `yaml:"interfacename"`
	KubeVipLoadbalancer string `yaml:"kubeviploadbalancer"`
	KIBTimeout          string `yaml:"kibtimeout"`
	PivotTimeout        string `yaml:"pivottimeout"`
	PodSubnet           string `yaml:"podsubnet"`
	ServiceSubnet       string `yaml:"servicesubnet"`
	MetalAddressRange   string `yaml:"metaladdressrange"`
}
type Registry struct {
	Host          string `yaml:"host,omitempty"`
	Username      string `yaml:"username,omitempty"`
	Password      string `yaml:"password,omitempty"`
	Auth          string `yaml:"auth,omitempty"`
	IdentityToken string `yaml:"identityToken,omitempty"`
}

type Inventory struct {
	All struct {
		Vars struct {
			AnsibleUser              string `yaml:"ansible_user"`
			AnsiblePort              int    `yaml:"ansible_port"`
			AnsibleSSHPrivateKeyFile string `yaml:"ansible_ssh_private_key_file"`
		} `yaml:"vars"`
		Hosts map[string]*AnsibleHost `yaml:"hosts"`
	} `yaml:"all"`
}

type AnsibleHost struct {
	AnsibleHost string `yaml:"ansible_host"`
}

type KubeadmControlPlane struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		KubeadmConfigSpec struct {
			ClusterConfiguration struct {
				APIServer struct {
					ExtraArgs struct {
						AuditLogMaxage           string `yaml:"audit-log-maxage"`
						AuditLogMaxbackup        string `yaml:"audit-log-maxbackup"`
						AuditLogMaxsize          string `yaml:"audit-log-maxsize"`
						AuditLogPath             string `yaml:"audit-log-path"`
						AuditPolicyFile          string `yaml:"audit-policy-file"`
						CloudProvider            string `yaml:"cloud-provider"`
						EncryptionProviderConfig string `yaml:"encryption-provider-config"`
					} `yaml:"extraArgs"`
					ExtraVolumes []struct {
						HostPath  string `yaml:"hostPath"`
						MountPath string `yaml:"mountPath"`
						Name      string `yaml:"name"`
					} `yaml:"extraVolumes"`
				} `yaml:"apiServer"`
				ControllerManager struct {
					ExtraArgs struct {
						CloudProvider       string `yaml:"cloud-provider"`
						FlexVolumePluginDir string `yaml:"flex-volume-plugin-dir"`
					} `yaml:"extraArgs"`
				} `yaml:"controllerManager"`
				DNS struct {
				} `yaml:"dns"`
				Etcd struct {
					Local struct {
						ImageTag string `yaml:"imageTag"`
					} `yaml:"local"`
				} `yaml:"etcd"`
				Networking struct {
				} `yaml:"networking"`
				Scheduler struct {
				} `yaml:"scheduler"`
			} `yaml:"clusterConfiguration"`
			Files []struct {
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
			} `yaml:"files"`
			Format            string `yaml:"format"`
			InitConfiguration struct {
				LocalAPIEndpoint struct {
				} `yaml:"localAPIEndpoint"`
				NodeRegistration struct {
					CriSocket        string `yaml:"criSocket"`
					KubeletExtraArgs struct {
						CloudProvider   string `yaml:"cloud-provider"`
						VolumePluginDir string `yaml:"volume-plugin-dir"`
					} `yaml:"kubeletExtraArgs"`
				} `yaml:"nodeRegistration"`
			} `yaml:"initConfiguration"`
			JoinConfiguration struct {
				Discovery struct {
				} `yaml:"discovery"`
				NodeRegistration struct {
					CriSocket        string `yaml:"criSocket"`
					KubeletExtraArgs struct {
						CloudProvider   string `yaml:"cloud-provider"`
						VolumePluginDir string `yaml:"volume-plugin-dir"`
					} `yaml:"kubeletExtraArgs"`
				} `yaml:"nodeRegistration"`
			} `yaml:"joinConfiguration"`
			PreKubeadmCommands []string `yaml:"preKubeadmCommands"`
		} `yaml:"kubeadmConfigSpec"`
		MachineTemplate struct {
			InfrastructureRef struct {
				APIVersion string `yaml:"apiVersion"`
				Kind       string `yaml:"kind"`
				Name       string `yaml:"name"`
				Namespace  string `yaml:"namespace"`
			} `yaml:"infrastructureRef"`
			Metadata struct {
			} `yaml:"metadata"`
		} `yaml:"machineTemplate"`
		Replicas        int `yaml:"replicas"`
		RolloutStrategy struct {
			RollingUpdate struct {
				MaxSurge int `yaml:"maxSurge,omitempty"`
			} `yaml:"rollingUpdate,omitempty"`
			Type string `yaml:"type,omitempty"`
		} `yaml:"rolloutStrategy,omitempty"`
		Version string `yaml:"version"`
	} `yaml:"spec"`
}

type PreprovisionedMachineTemplate struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				InventoryRef struct {
					Name      string `yaml:"name"`
					Namespace string `yaml:"namespace"`
				} `yaml:"inventoryRef"`
				OverrideRef struct {
					Name string `yaml:"name"`
					//Namespace string `yaml:"namespace"`
				} `yaml:"overrideRef,omitempty"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type MachineDeployment struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Labels struct {
			ClusterXK8SIoClusterName string `yaml:"cluster.x-k8s.io/cluster-name"`
		} `yaml:"labels"`
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		ClusterName             string `yaml:"clusterName"`
		MinReadySeconds         int    `yaml:"minReadySeconds"`
		ProgressDeadlineSeconds int    `yaml:"progressDeadlineSeconds"`
		Replicas                int    `yaml:"replicas"`
		RevisionHistoryLimit    int    `yaml:"revisionHistoryLimit"`
		Selector                struct {
			MatchLabels struct {
				ClusterXK8SIoClusterName    string `yaml:"cluster.x-k8s.io/cluster-name"`
				ClusterXK8SIoDeploymentName string `yaml:"cluster.x-k8s.io/deployment-name"`
			} `yaml:"matchLabels"`
		} `yaml:"selector"`
		Strategy struct {
			RollingUpdate struct {
				MaxSurge       int `yaml:"maxSurge"`
				MaxUnavailable int `yaml:"maxUnavailable"`
			} `yaml:"rollingUpdate"`
			Type string `yaml:"type"`
		} `yaml:"strategy"`
		Template struct {
			Metadata struct {
				Labels struct {
					ClusterXK8SIoClusterName    string `yaml:"cluster.x-k8s.io/cluster-name"`
					ClusterXK8SIoDeploymentName string `yaml:"cluster.x-k8s.io/deployment-name"`
				} `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Bootstrap struct {
					ConfigRef struct {
						APIVersion string `yaml:"apiVersion"`
						Kind       string `yaml:"kind"`
						Name       string `yaml:"name"`
					} `yaml:"configRef"`
				} `yaml:"bootstrap"`
				ClusterName       string `yaml:"clusterName"`
				InfrastructureRef struct {
					APIVersion string `yaml:"apiVersion"`
					Kind       string `yaml:"kind"`
					Name       string `yaml:"name"`
				} `yaml:"infrastructureRef"`
				Version string `yaml:"version"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type KubeadmConfigTemplate struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Files []struct {
					Content     string `yaml:"content"`
					Path        string `yaml:"path"`
					Permissions string `yaml:"permissions"`
				} `yaml:"files"`
				Format            string `yaml:"format"`
				JoinConfiguration struct {
					NodeRegistration struct {
						CriSocket        string `yaml:"criSocket"`
						KubeletExtraArgs struct {
							CloudProvider   string `yaml:"cloud-provider"`
							VolumePluginDir string `yaml:"volume-plugin-dir"`
						} `yaml:"kubeletExtraArgs"`
					} `yaml:"nodeRegistration"`
				} `yaml:"joinConfiguration"`
				PreKubeadmCommands  []string `yaml:"preKubeadmCommands"`
				PostKubeadmCommands []string `yaml:"postKubeadmCommands,omitempty"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type mlbConfigMap struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"metadata"`
	Data struct {
		Config string `yaml:"config"`
	} `yaml:"data"`
}

type capiCluster struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Labels struct {
			KonvoyD2IqIoClusterName  string `yaml:"konvoy.d2iq.io/cluster-name"`
			KonvoyD2IqIoCni          string `yaml:"konvoy.d2iq.io/cni"`
			KonvoyD2IqIoCsi          string `yaml:"konvoy.d2iq.io/csi"`
			KonvoyD2IqIoLoadbalancer string `yaml:"konvoy.d2iq.io/loadbalancer"`
			KonvoyD2IqIoOsHint       string `yaml:"konvoy.d2iq.io/osHint"`
			KonvoyD2IqIoProvider     string `yaml:"konvoy.d2iq.io/provider"`
		} `yaml:"labels"`
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		ClusterNetwork struct {
			Pods struct {
				CidrBlocks []string `yaml:"cidrBlocks"`
			} `yaml:"pods"`
			Services struct {
				CidrBlocks []string `yaml:"cidrBlocks"`
			} `yaml:"services"`
		} `yaml:"clusterNetwork"`
		ControlPlaneEndpoint struct {
			Host string `yaml:"host"`
			Port int    `yaml:"port"`
		} `yaml:"controlPlaneEndpoint"`
		ControlPlaneRef struct {
			APIVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
			Name       string `yaml:"name"`
			Namespace  string `yaml:"namespace"`
		} `yaml:"controlPlaneRef"`
		InfrastructureRef struct {
			APIVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
			Name       string `yaml:"name"`
			Namespace  string `yaml:"namespace"`
		} `yaml:"infrastructureRef"`
	} `yaml:"spec"`
}

//this class is used to read in a generic k8s object from the dry run output. We don't know what it will be until we look
type k8sObject struct {
	APIVersion string                 `yaml:"apiVersion,omitempty"`
	Kind       string                 `yaml:"kind,omitempty"`
	Metadata   map[string]interface{} `yaml:"metadata,omitempty"`
	Spec       map[string]interface{} `yaml:"spec,omitempty"`
	Data       map[string]interface{} `yaml:"data,omitempty"`
}

type kibOverride struct {
	Gpu struct {
		Types []string `yaml:"types,omitempty"`
	} `yaml:"gpu,omitempty"`
	BuildNameExtra          string `yaml:"build_name_extra,omitempty"`
	ImageRegistriesWithAuth []struct {
		Host          string `yaml:"host,omitempty"`
		Username      string `yaml:"username,omitempty"`
		Password      string `yaml:"password,omitempty"`
		Auth          string `yaml:"auth,omitempty"`
		IdentityToken string `yaml:"identityToken,omitempty"`
	} `yaml:"image_registries_with_auth,omitempty"`
	OsPackagesLocalBundleFile  string `yaml:"os_packages_local_bundle_file,omitempty"`
	PipPackagesLocalBundleFile string `yaml:"pip_packages_local_bundle_file,omitempty"`
	ImagesLocalBundleDir       string `yaml:"images_local_bundle_dir,omitempty"`
}
