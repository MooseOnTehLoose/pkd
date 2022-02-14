package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"

	"gopkg.in/yaml.v3"
)

type NodePool struct {
	Hosts map[string]string
	Flags map[string]bool
}

type ControlPlane struct {
	Loadbalancer string
	Hosts        map[string]string
	Flags        map[string]bool
}

type MetaData struct {
	Name          string
	SshUser       string
	SshPrivateKey string
	InterfaceName string
}

type Registry struct {
	Address  string
	User     string
	Password string
}

type Cluster struct {
	MetaData     MetaData
	Registry     Registry
	Controlplane ControlPlane
	NodePools    map[string]NodePool
}

type RegistryOverride struct {
	ImageRegistriesWithAuth []struct {
		Host          string `yaml:"host"`
		Username      string `yaml:"username"`
		Password      string `yaml:"password"`
		Auth          string `yaml:"auth"`
		IdentityToken string `yaml:"identityToken"`
	} `yaml:"image_registries_with_auth"`
}

type GpuOverride struct {
	Gpu struct {
		Types []string `yaml:"types"`
	} `yaml:"gpu"`
	BuildNameExtra string `yaml:"build_name_extra"`
}

type GpuRegOverride struct {
	Gpu struct {
		Types []string `yaml:"types"`
	} `yaml:"gpu"`
	BuildNameExtra          string `yaml:"build_name_extra"`
	ImageRegistriesWithAuth []struct {
		Host          string `yaml:"host"`
		Username      string `yaml:"username"`
		Password      string `yaml:"password"`
		Auth          string `yaml:"auth"`
		IdentityToken string `yaml:"identityToken"`
	} `yaml:"image_registries_with_auth"`
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
					Name      string `yaml:"name"`
					Namespace string `yaml:"namespace"`
				} `yaml:"overrideRef,omitempty"`
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
				JoinConfiguration struct {
					NodeRegistration struct {
						CriSocket        string `yaml:"criSocket"`
						KubeletExtraArgs struct {
							CloudProvider   string `yaml:"cloud-provider"`
							VolumePluginDir string `yaml:"volume-plugin-dir"`
						} `yaml:"kubeletExtraArgs"`
					} `yaml:"nodeRegistration"`
				} `yaml:"joinConfiguration"`
				PreKubeadmCommands []string `yaml:"preKubeadmCommands"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

/////////////////////////////////////////////////////////////////////////////////////////////////
// pkd args:
//
// pkd 						prints usage
// pkd init					create cluster.yaml and kommander.yaml templates
// pkd up [dry-run]			create all yaml resources needed to deploy a cluster
// pkd bootstrap [up/down]	control the bootstrap cluster
// pkd apply				apply all resources created with dry-run
// pkd pivot				pivot the controllers and resources to the cluster from bootstrap
// pkd merge [kubeconfig]	merge kubeconfig
//
///////////////////////////////////////////////////////////////////////////////////////////////
func main() {
	argNum := len(os.Args)

	if argNum >= 2 {
		arg1 := os.Args[1]

		switch {
		//init
		case arg1 == "init":
			fmt.Printf("generating cluster.yaml")
			initYaml()
		//up
		case arg1 == "up":
			if argNum >= 3 && os.Args[2] == "dry-run" {
				fmt.Printf("Dry Run! Creating resources to be applied via pkd apply")
			} else {
				up()
			}
		//bootstrap
		case arg1 == "bootstrap":
			//If no 2nd arg given, just create the bootstrap.
			//If given, either create or delete the bootstrap
			if argNum == 2 {
				fmt.Printf("Creating bootstrap cluster")
				bootstrap("up")
			} else if argNum == 3 {
				if os.Args[2] == "up" {
					fmt.Printf("Creating bootstrap cluster")
					bootstrap("up")
				} else if os.Args[2] == "down" {
					fmt.Printf("Deleting bootstrap cluster")
					bootstrap("down")
				} else {
					fmt.Printf("Error, Arg invalid. Usage:")
				}
			}
		//apply
		case arg1 == "apply":
			fmt.Printf("Applying cluster resources to deploy cluster")
		//pivot
		case arg1 == "pivot":
			fmt.Printf("Pivoting cluster from boostrap")
		//merge
		case arg1 == "merge":
			fmt.Printf("merging kubeconfig into ~/.kube/config")
		//no args or bad args
		default:
			fmt.Printf("Usage:\n" +
				" pkd 						prints usage\n" +
				" pkd init					create cluster.yaml and kommander.yaml templates\n" +
				" pkd up [dry-run]			create all yaml resources needed to deploy a cluster\n" +
				" pkd bootstrap [up/down]	control the bootstrap cluster\n" +
				" pkd apply				apply all resources created with dry-run\n" +
				" pkd pivot				pivot the controllers and resources to the cluster from bootstrap\n" +
				"/ pkd merge [kubeconfig]	merge kubeconfig\n")
		}

	}
}

//Read in cluster.yaml and start the cluster creation process
func up() {

	fmt.Printf("Deploying DKP Cluster\n")

	//haven't tested bootstrap command yet
	//bootstrap("down")
	//bootstrap("up")

	cluster := loadCluster()

	//haven't tested this yet
	//these need error handling
	//exec.Command("kubectl create secret generic " + cluster.MetaData.Name + "-ssh-key --from-file=ssh-privatekey=" + cluster.MetaData.SshPrivateKey)
	//exec.Command("kubectl label secret " + cluster.MetaData.Name + "-ssh-key clusterctl.cluster.x-k8s.io/move=")

	//Create a ControlPlane PreProvisionedInventory Ojbect
	genCPPI(cluster.MetaData, "controlplane", cluster.Controlplane)

	//For Each NodePool, create a Preprovisioned Inventory Object
	//mdval sets the machinedeployment name ie md-0
	mdVal := 0
	for nodesetName, nodes := range cluster.NodePools {
		genPPI(cluster.MetaData, nodesetName, nodes, mdVal)
		mdVal++
	}

	//We only need to count the number of replicas in the 1st node pool at this time

	genOverride(cluster.Registry)

	//haven't tested this
	//Generate the cluster.yaml
	//exec.Command(
	//	"./dkp create cluster preprovisioned --cluster-name " +
	//		cluster.MetaData.Name + "--control-plane-endpoint-host " +
	//		cluster.Controlplane.Loadbalancer + " --virtual-ip-interface " +
	//		cluster.MetaData.InterfaceName + " --worker-replicas " +
	//		numReplicas(cluster.NodePools["md-0"]) + " --dry-run -o yaml > " +
	//		cluster.MetaData.Name + ".yaml")

	for nodesetName, nodes := range cluster.NodePools {
		if nodesetName != "md-0" {

			md := MachineDeployment{}
			md.APIVersion = "cluster.x-k8s.io/v1alpha4"
			md.Kind = "MachineDeployment"
			md.Metadata.Labels.ClusterXK8SIoClusterName = cluster.MetaData.Name
			md.Metadata.Name = cluster.MetaData.Name + "-" + nodesetName
			md.Metadata.Namespace = "default"
			md.Spec.ClusterName = cluster.MetaData.Name
			md.Spec.MinReadySeconds = 0
			md.Spec.ProgressDeadlineSeconds = 600
			md.Spec.Replicas = len(nodes.Hosts)
			md.Spec.RevisionHistoryLimit = 1
			md.Spec.Selector.MatchLabels.ClusterXK8SIoClusterName = cluster.MetaData.Name
			md.Spec.Selector.MatchLabels.ClusterXK8SIoDeploymentName = cluster.MetaData.Name + "-" + nodesetName
			md.Spec.Strategy.RollingUpdate.MaxSurge = 1
			md.Spec.Strategy.RollingUpdate.MaxUnavailable = 0
			md.Spec.Strategy.Type = "RollingUpdate"
			md.Spec.Template.Metadata.Labels.ClusterXK8SIoClusterName = cluster.MetaData.Name
			md.Spec.Template.Metadata.Labels.ClusterXK8SIoDeploymentName = cluster.MetaData.Name + "-" + nodesetName
			md.Spec.Template.Spec.Bootstrap.ConfigRef.APIVersion = "bootstrap.cluster.x-k8s.io/v1alpha4"
			md.Spec.Template.Spec.Bootstrap.ConfigRef.Kind = "KubeadmConfigTemplate"
			md.Spec.Template.Spec.Bootstrap.ConfigRef.Name = cluster.MetaData.Name + "-" + nodesetName
			md.Spec.Template.Spec.ClusterName = cluster.MetaData.Name
			md.Spec.Template.Spec.InfrastructureRef.APIVersion = "infrastructure.cluster.konvoy.d2iq.io/v1alpha1"
			md.Spec.Template.Spec.InfrastructureRef.Kind = "PreprovisionedMachineTemplate"
			md.Spec.Template.Spec.InfrastructureRef.Name = cluster.MetaData.Name + "-" + nodesetName
			md.Spec.Template.Spec.Version = "v1.21.6"

			pmt := PreprovisionedMachineTemplate{}
			pmt.APIVersion = "infrastructure.cluster.konvoy.d2iq.io/v1alpha1"
			pmt.Kind = "PreprovisionedMachineTemplate"
			pmt.Metadata.Name = cluster.MetaData.Name + "-" + nodesetName
			pmt.Metadata.Namespace = "default"
			pmt.Spec.Template.Spec.InventoryRef.Name = cluster.MetaData.Name + "-" + nodesetName
			pmt.Spec.Template.Spec.InventoryRef.Namespace = "default"
			switch {
			case nodes.Flags["registry"] && nodes.Flags["gpu"]:
				pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-" + nodesetName + "-gpuRegOverride"
				pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
			case nodes.Flags["registry"]:
				pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-" + nodesetName + "-registryOverride"
				pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
			case nodes.Flags["gpu"]:
				pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-" + nodesetName + "-gpuOverride"
				pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"

			}

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
			kct.APIVersion = "bootstrap.cluster.x-k8s.io/v1alpha4"
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
			kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.CriSocket = "/run/containerd/containerd.sock"
			kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.CloudProvider = ""
			kct.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs.VolumePluginDir = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
			kct.Spec.Template.Spec.PreKubeadmCommands = append(kct.Spec.Template.Spec.PreKubeadmCommands,
				"systemctl daemon-reload",
				"systemctl restart containerd",
				"/run/kubeadm/konvoy-set-kube-proxy-configuration.sh")

			data, err := yaml.Marshal(&md)
			if err != nil {
				log.Fatal(err)
			}
			err = ioutil.WriteFile(nodesetName+"-MachineDeployment.yaml", data, 0644)
			if err != nil {
				log.Fatal(err)
			}
			data, err = yaml.Marshal(&pmt)
			if err != nil {
				log.Fatal(err)
			}
			err = ioutil.WriteFile(nodesetName+"-PreprovisionedMachineTemplate.yaml", data, 0644)
			if err != nil {
				log.Fatal(err)
			}
			data, err = yaml.Marshal(&kct)
			if err != nil {
				log.Fatal(err)
			}
			err = ioutil.WriteFile(nodesetName+"-KubeadmConfigTemplate.yaml", data, 0644)
			if err != nil {
				log.Fatal(err)
			}

		}
	}
}

//todo: only generate the overrides we actually need, waiting till I build out the rest of this
func genOverride(registryInfo Registry) {

	//registry
	registryOverride := RegistryOverride{
		ImageRegistriesWithAuth: []struct {
			Host          string "yaml:\"host\""
			Username      string "yaml:\"username\""
			Password      string "yaml:\"password\""
			Auth          string "yaml:\"auth\""
			IdentityToken string "yaml:\"identityToken\""
		}{
			{
				Host:     registryInfo.Address,
				Username: registryInfo.User,
				Password: registryInfo.Password,
			},
		},
	}

	gpuOverride := GpuOverride{
		Gpu: struct {
			Types []string "yaml:\"types\""
		}{
			Types: []string{
				"nvidia",
			},
		},
		BuildNameExtra: "\"-nvidia\"",
	}

	//gpu and registry
	gpuRegOverride := GpuRegOverride{
		Gpu: struct {
			Types []string "yaml:\"types\""
		}{
			Types: []string{
				"nvidia",
			},
		},
		BuildNameExtra: "\"-nvidia\"",
		ImageRegistriesWithAuth: []struct {
			Host          string "yaml:\"host\""
			Username      string "yaml:\"username\""
			Password      string "yaml:\"password\""
			Auth          string "yaml:\"auth\""
			IdentityToken string "yaml:\"identityToken\""
		}{
			{
				Host:     registryInfo.Address,
				Username: registryInfo.User,
				Password: registryInfo.Password,
			},
		},
	}

	data, err := yaml.Marshal(&registryOverride)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("registryOverride.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	data, err = yaml.Marshal(&gpuOverride)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("gpuOverride.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	data, err = yaml.Marshal(&gpuRegOverride)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("gpuRegOverride.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

}

func genCPPI(mdata MetaData, clusterName string, cplane ControlPlane) {

	//initialize the array
	hosts := make([]map[string]string, len(cplane.Hosts))
	index := 0
	//create the array of hosts
	for _, ip := range cplane.Hosts {
		hosts[index] = map[string]string{"address": ip}
		index++
	}

	ppi := map[string]interface{}{
		"apiVersion": "infrastructure.cluster.konvoy.d2iq.io/v1alpha1",
		"kind":       "PreprovisionedInventory",
		"metadata": map[string]interface{}{
			"name":      mdata.Name + "-control-plane",
			"namespace": "default",
			"labels": map[string]string{
				"cluster.x-k8s.io/cluster-name":    mdata.Name,
				"clusterctl.cluster.x-k8s.io/move": "",
			},
		},
		"spec": map[string]interface{}{
			"hosts": hosts,
			"sshConfig": map[string]interface{}{
				"port": 22,
				"user": mdata.SshUser,
				"privateKeyRef": map[string]string{
					"name":      mdata.SshPrivateKey,
					"namespace": "default",
				},
			},
		},
	}

	data, err := yaml.Marshal(&ppi)

	if err != nil {
		log.Fatal(err)
	}

	err2 := ioutil.WriteFile(mdata.Name+"-control-plane.yaml", data, 0644)

	if err2 != nil {

		log.Fatal(err2)
	}

}

func genPPI(mdata MetaData, clusterName string, npool NodePool, mdVal int) {

	//initialize the array
	hosts := make([]map[string]string, len(npool.Hosts))
	index := 0
	//create the array of hosts
	for _, ip := range npool.Hosts {
		hosts[index] = map[string]string{"address": ip}
		index++
	}

	ppi := map[string]interface{}{
		"apiVersion": "infrastructure.cluster.konvoy.d2iq.io/v1alpha1",
		"kind":       "PreprovisionedInventory",
		"metadata": map[string]interface{}{
			"name":      mdata.Name + "-md-" + strconv.Itoa(mdVal),
			"namespace": "default",
			"labels": map[string]string{
				"cluster.x-k8s.io/cluster-name":    mdata.Name,
				"clusterctl.cluster.x-k8s.io/move": "",
			},
		},
		"spec": map[string]interface{}{
			"hosts": hosts,
			"sshConfig": map[string]interface{}{
				"port": 22,
				"user": mdata.SshUser,
				"privateKeyRef": map[string]string{
					"name":      mdata.SshPrivateKey,
					"namespace": "default",
				},
			},
		},
	}

	data, err := yaml.Marshal(&ppi)

	if err != nil {
		log.Fatal(err)
	}

	err2 := ioutil.WriteFile(mdata.Name+"-md-"+strconv.Itoa(mdVal)+".yaml", data, 0644)

	if err2 != nil {

		log.Fatal(err2)
	}

}

func loadCluster() Cluster {
	clusterYaml, err := ioutil.ReadFile("cluster.yaml")

	if err != nil {

		log.Fatal(err)
	}
	data := Cluster{
		MetaData:     MetaData{},
		Registry:     Registry{},
		Controlplane: ControlPlane{},
		NodePools:    map[string]NodePool{},
	}

	err2 := yaml.Unmarshal(clusterYaml, &data)

	if err2 != nil {

		log.Fatal(err2)
	}

	return data

}

//Generate a YAML file containing some default values for a DKP CLuster
func initYaml() {
	initcluster := Cluster{
		MetaData: MetaData{
			Name:          "cluster-name",
			SshUser:       "user",
			SshPrivateKey: "id_rsa",
			InterfaceName: "ens192",
		},
		Registry: Registry{
			Address:  "io.dockerhub.com",
			User:     "user",
			Password: "password",
		},
		Controlplane: ControlPlane{
			Loadbalancer: "10.0.0.10",
			Hosts:        map[string]string{"controlplane1": "10.0.0.11", "controlplane2": "10.0.0.12", "controlplane3": "10.0.0.13"},
			Flags:        map[string]bool{"registry": true},
		},
		NodePools: map[string]NodePool{
			"md-0": {
				Hosts: map[string]string{"worker1": "10.0.0.14", "worker2": "10.0.0.15", "worker3": "10.0.0.16", "worker4": "10.0.0.17"},
				Flags: map[string]bool{"registry": true},
			},
			"md-1": {
				Hosts: map[string]string{"worker1": "10.0.0.18", "worker2": "10.0.0.19"},
				Flags: map[string]bool{"registry": true, "gpu": true},
			},
		},
	}

	data, err := yaml.Marshal(&initcluster)

	if err != nil {
		log.Fatal(err)
	}

	err2 := ioutil.WriteFile("cluster.yaml", data, 0)

	if err2 != nil {

		log.Fatal(err2)
	}

}

//have not tested this yet
//Start the bootstrap cluster on this host
func bootstrap(str string) {
	if str == "up" {
		fmt.Printf("Creating Bootstrap Cluster")

		exec.Command("./dkp create bootstrap")
	} else if str == "down" {
		fmt.Printf("Deleting Bootstrap Cluster")

		exec.Command("./dkp delete bootstrap")
	}
}

//have not tested this yet
//func createOverrideSecret(name string, override string) {
//	exec.Command("kubectl", "create secret generic "+name+"-"+override+"-override --from-file="+override+".yaml="+override+".yaml")
//	exec.Command("kubectl", "label secret "+name+"-"+override+"-override clusterctl.cluster.x-k8s.io/move=")
//}
