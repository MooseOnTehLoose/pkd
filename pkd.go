package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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
	Host          string `yaml:"host"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	Auth          string `yaml:"auth"`
	IdentityToken string `yaml:"identityToken"`
}

type Cluster struct {
	MetaData     MetaData
	Registry     Registry
	Controlplane ControlPlane
	NodePools    map[string]NodePool
}

type RegistryOverride struct {
	ImageRegistriesWithAuth []Registry `yaml:"image_registries_with_auth"`
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
			fmt.Printf("Generating cluster.yaml")
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
				" pkd apply					apply all resources created with dry-run\n" +
				" pkd pivot					pivot the controllers and resources to the cluster from bootstrap\n" +
				"/ pkd merge [kubeconfig]	merge kubeconfig\n")
		}

	}
}

//Read in cluster.yaml and start the cluster creation process
func up() {

	//We need to generate the folder to store our k8s objects after creation
	os.MkdirAll("resources", os.ModePerm)
	fmt.Printf("Created resources directory\n")
	os.MkdirAll("overrides", os.ModePerm)
	fmt.Printf("Created overrides directory\n")

	bootstrap("down")
	bootstrap("up")

	//This loads the user customizable values to generate a cluster from cluster.yaml
	cluster := loadCluster()

	fmt.Printf("Cluster YAML loaded into PKD\n")

	createSSHSecret(cluster.MetaData.Name, cluster.MetaData.SshPrivateKey)
	fmt.Printf("Created SSH Secret\n")

	//Create a ControlPlane PreProvisionedInventory Ojbect
	genCPPI(cluster.MetaData, "controlplane", cluster.Controlplane)
	fmt.Printf("Generated Contorl Plane PPI\n")
	//For Each NodePool, create a Preprovisioned Inventory Object
	//mdval sets the machinedeployment name ie md-0
	mdVal := 0
	for nodesetName, nodes := range cluster.NodePools {
		genPPI(cluster.MetaData, nodesetName, nodes, mdVal)
		fmt.Printf("Generated md-" + strconv.Itoa(mdVal) + " PPI\n")
		mdVal++
	}

	//apply the PreProvisionedInventory objects to the bootstrap cluster
	applyPPI(cluster.MetaData.Name)
	fmt.Printf("Applied all PPI\n")

	//Generate the cluster.yaml dry run output
	fmt.Printf(cluster.MetaData.Name + " \n" + cluster.Controlplane.Loadbalancer + " \n" + cluster.MetaData.InterfaceName + " \n")
	dkpDryRun(cluster.MetaData.Name, cluster.Controlplane.Loadbalancer, cluster.MetaData.InterfaceName)
	fmt.Printf("Dry Run Completed\n")

	//Read in the dry run output
	dryRunYaml, err := ioutil.ReadFile(cluster.MetaData.Name + ".yaml")
	if err != nil {
		log.Fatal(err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(dryRunYaml))
	var i map[string]interface{}
	for decoder.Decode(&i) == nil {

		resourceName := i["metadata"].(map[string]interface{})["name"].(string)
		resourceKind := i["kind"].(string)

		if !strings.Contains(resourceName, "md-0") {
			fileName := "resources/" + resourceName + "-" + resourceKind + ".yaml"
			file, err := yaml.Marshal(&i)
			if err != nil {
				log.Fatal(err)
			}
			err = ioutil.WriteFile(fileName, file, 0644)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	fmt.Printf("Generated the Resource Objects from Dry Run Output File\n")

	flagEnabled := map[string]bool{"registry": false, "gpu": false, "registryGPU": false}

	//Create all MachineDeployment, PreProvisionedMachineTemplate and KubeadmConfigTemplate objects
	//Also trigger the overrides
	for nodesetName, nodes := range cluster.NodePools {
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
			pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-gpu-registry-override"
			pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
			flagEnabled["registryGPU"] = true
		case nodes.Flags["registry"]:
			pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-registry-override"
			pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
			flagEnabled["registry"] = true
		case nodes.Flags["gpu"]:
			pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-gpu-override"
			pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
			flagEnabled["gpu"] = true

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
		err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-MachineDeployment.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
		data, err = yaml.Marshal(&pmt)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-PreprovisionedMachineTemplate.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
		data, err = yaml.Marshal(&kct)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-KubeadmConfigTemplate.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}

	}
	fmt.Printf("Generated all Custom Resources for NodePools\n")

	//creates all override files for KIB
	genOverride(cluster.MetaData.Name, cluster.Registry, flagEnabled)
	fmt.Printf("Generated All Overrides\n")
	//apply all resources to the cluster
	applyResources(cluster.MetaData.Name)
	fmt.Printf("Applied All Resources, Cluster Spinning Up\n")
	//kubectl  wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
	waitForClusterReady(cluster.MetaData.Name)
	fmt.Printf("Cluster Is Ready\n")
	//#Get the kubeconfig for our new cluster
	getKubeconfig(cluster.MetaData.Name)
	fmt.Printf("Grabbed the Kubeconfig\n")
	//#Pivot to the new cluster
	pivotCluster(cluster.MetaData.Name)
	fmt.Printf("Pivoted the Cluster\n")
	//#Clean up the Bootstrap Cluster
	bootstrap("down")
	fmt.Printf("Cleaned up the Bootstrap Cluster\n")
	//#Merge the kubeconfig into our ~/.kube/config
	mergeKubeconfig(cluster.MetaData.Name)
	fmt.Printf("Merged the Kubeconfig\n")
}

//todo: only generate the overrides we actually need, waiting till I build out the rest of this
func genOverride(clusterName string, registryInfo Registry, flags map[string]bool) {

	//registry
	if flags["registry"] {
		registryOverride := RegistryOverride{}
		registryOverride.ImageRegistriesWithAuth = append(registryOverride.ImageRegistriesWithAuth, registryInfo)

		data, err := yaml.Marshal(&registryOverride)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("overrides/registryOverride.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}

		//#Create Override secret for Bootstrap Cluster
		//kubectl create secret generic $CLUSTER_NAME-overrides --from-file=overrides.yaml=overrides.yaml
		//kubectl label secret $CLUSTER_NAME-overrides clusterctl.cluster.x-k8s.io/move=

		cmd := exec.Command("kubectl", "create", "secret", "generic", clusterName+"-registry-override", "--from-file=overrides.yaml=overrides/registryOverride.yaml")
		//run the command
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("kubectl", "label", "secret", clusterName+"-registry-override", "clusterctl.cluster.x-k8s.io/move=")
		//run the command
		output, err = cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
	}

	if flags["gpu"] {
		gpuOverride := GpuOverride{}
		gpuOverride.Gpu.Types = append(gpuOverride.Gpu.Types, "nvidia")
		gpuOverride.BuildNameExtra = "-nvidia"
		data, err := yaml.Marshal(&gpuOverride)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("overrides/gpuOverride.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
		cmd := exec.Command("kubectl", "create", "secret", "generic", clusterName+"-gpu-override", "--from-file=overrides.yaml=overrides/gpuOverride.yaml")
		//run the command
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("kubectl", "label", "secret", clusterName+"-gpu-override", "clusterctl.cluster.x-k8s.io/move=")
		//run the command
		output, err = cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
	}

	if flags["registryGPU"] {
		//gpu and registry
		gpuRegOverride := GpuRegOverride{}
		gpuRegOverride.Gpu.Types = append(gpuRegOverride.Gpu.Types, "nvidia")
		gpuRegOverride.BuildNameExtra = "-nvidia"
		gpuRegOverride.ImageRegistriesWithAuth = append(gpuRegOverride.ImageRegistriesWithAuth, registryInfo)

		data, err := yaml.Marshal(&gpuRegOverride)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("overrides/gpuRegOverride.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}
		cmd := exec.Command("kubectl", "create", "secret", "generic", clusterName+"-gpu-registry-override", "--from-file=overrides.yaml=overrides/gpuRegOverride.yaml")
		//run the command
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
		cmd = exec.Command("kubectl", "label", "secret", clusterName+"-gpu-registry-override", "clusterctl.cluster.x-k8s.io/move=")
		//run the command
		output, err = cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
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

	err2 := ioutil.WriteFile("resources/"+mdata.Name+"-control-plane-PreprovisionedInventory.yaml", data, 0644)

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

	err2 := ioutil.WriteFile("resources/"+mdata.Name+"-md-"+strconv.Itoa(mdVal)+"-PreprovisionedInventory.yaml", data, 0644)

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
			Host:          "",
			Username:      "",
			Password:      "",
			Auth:          "",
			IdentityToken: "",
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

	err2 := ioutil.WriteFile("cluster.yaml", data, 0644)

	if err2 != nil {

		log.Fatal(err2)
	}

}

//have not tested this yet
//Start the bootstrap cluster on this host
func bootstrap(str string) {
	if str == "up" {
		fmt.Printf("Creating Bootstrap Cluster\n")

		cmd := exec.Command("./dkp", "create", "bootstrap")
		//run the command
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}

	} else if str == "down" {
		fmt.Printf("Deleting Bootstrap Cluster\n")

		cmd := exec.Command("./dkp", "delete", "bootstrap")
		//run the command
		output, err := cmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			log.Fatal(err)
		}
	}
}

func mergeKubeconfig(clusterName string) {

	os.Setenv("KUBECONFIG", "./"+clusterName+"+.conf:"+os.Getenv("HOME")+"/.kube/config")

	cmd1 := exec.Command("kubectl", "config", "view", "--flatten")
	cmd1.Env = os.Environ()

	// open the out file for writing
	mergedKubeconfig, err := os.Create("./merged.conf")
	if err != nil {
		panic(err)
	}
	defer mergedKubeconfig.Close()
	cmd1.Stdout = mergedKubeconfig
	err = cmd1.Run()
	if err != nil {
		log.Fatal(err)
	}

	err = os.Chmod("merged.conf", 0600)
	if err != nil {
		log.Fatal(err)
	}

	start := "merged.conf"
	destination := os.Getenv("HOME") + "/.kube/config"
	os.Rename(start, destination)

	cmd2 := exec.Command("kubectl", "config", "get-contexts", "--kubeconfig=./"+clusterName+".conf", "--output=name")

	var outbuf, errbuf strings.Builder
	cmd2.Stdout = &outbuf
	cmd2.Stderr = &errbuf
	err = cmd2.Run()
	if err != nil {
		log.Fatal(err)
	}

	context := strings.TrimSuffix(outbuf.String(), "\n")
	os.Setenv("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")

	cmd3 := exec.Command("kubectl", "config", "set-context", context)
	cmd3.Env = os.Environ()

	err = cmd3.Run()
	if err != nil {
		log.Fatal(err)
	}

}

func getKubeconfig(clusterName string) {
	//create the command
	//./dkp get kubeconfig -c ${CLUSTER_NAME} > ${CLUSTER_NAME}.conf
	cmd := exec.Command("./dkp", "get", "kubeconfig", "-c", clusterName)

	//create the empty target file
	kubeconfig, err := os.Create(clusterName + ".conf")
	if err != nil {
		panic(err)
	}
	defer kubeconfig.Close()

	//dump the contents of our command into our empty file
	cmd.Stdout = kubeconfig

	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}

func createSSHSecret(clusterName string, keyName string) {

	//exec.Command("kubectl create secret generic " + cluster.MetaData.Name + "-ssh-key --from-file=ssh-privatekey=" + cluster.MetaData.SshPrivateKey)
	//exec.Command("kubectl label secret " + cluster.MetaData.Name + "-ssh-key clusterctl.cluster.x-k8s.io/move=")

	cmd := exec.Command("kubectl", "create", "secret", "generic", clusterName+"-ssh-key", "--from-file=ssh-privatekey="+keyName)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	cmd2 := exec.Command("kubectl", "label", "secret", clusterName+"-ssh-key", "clusterctl.cluster.x-k8s.io/move=")
	err = cmd2.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func applyPPI(clusterName string) {

	err := filepath.Walk("./resources/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		//cluster-a-control-plane-PreprovisionedInventory.yaml
		//
		//
		if strings.Contains(path, "-PreprovisionedInventory.yaml") && strings.Contains(path, clusterName) {
			//kubectl apply -f <cluster-name>-PreProvisionedInventory.yaml
			cmd := exec.Command("kubectl", "apply", "-f", path)
			//run the command
			err := cmd.Run()
			if err != nil {
				log.Fatal(err)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}

func dkpDryRun(clusterName string, clusterLoadBalancer string, interfaceName string) {

	cmd := exec.Command(
		"./dkp", "create", "cluster", "preprovisioned",
		"--cluster-name", clusterName,
		"--control-plane-endpoint-host", clusterLoadBalancer,
		"--virtual-ip-interface", interfaceName,
		"--dry-run", "-o", "yaml")

	//create the empty target file
	clusteryaml, err := os.Create(clusterName + ".yaml")
	if err != nil {
		panic(err)
	}
	defer clusteryaml.Close()

	//dump the contents of our command into our empty file
	cmd.Stdout = clusteryaml
	err = cmd.Run()

	if err != nil {
		log.Fatal(err)
	}
}
func applyResources(clusterName string) {
	err := filepath.Walk("./resources/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if !strings.Contains(path, "-PreprovisionedInventory.yaml") && strings.Contains(path, clusterName) {
			//kubectl apply -f <cluster-name>-PreProvisionedInventory.yaml
			cmd := exec.Command("kubectl", "apply", "-f", path)
			//run the command
			output, err := cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				log.Fatal(err)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}

func waitForClusterReady(clusterName string) {

	//kubectl  wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
	cmd := exec.Command("kubectl", "wait", "--for=condition=Ready", "clusters/"+clusterName, "--timeout=40m")

	//run the command
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}

func pivotCluster(clusterName string) {
	//#Pivot to the new cluster

	//./dkp create bootstrap controllers --kubeconfig ${CLUSTER_NAME}.conf
	cmd := exec.Command("./dkp", "create", "bootstrap", "controllers", "--kubeconfig", clusterName+".conf")

	//run the command
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	//./dkp move --to-kubeconfig ${CLUSTER_NAME}.conf
	cmd = exec.Command("./dkp", "move", "--to-kubeconfig", clusterName+".conf")

	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	//kubectl --kubeconfig ${CLUSTER_NAME}.conf wait --for=condition=ControlPlaneReady "clusters/${CLUSTER_NAME}" --timeout=20m
	cmd = exec.Command("kubectl", "--kubeconfig", clusterName+".conf", "wait", "--for=condition=ControlPlaneReady", "clusters/"+clusterName, "--timeout=20m")

	//run the command
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	//kubectl  wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
	cmd = exec.Command("kubectl", "wait", "--for=condition=Ready", "clusters/"+clusterName, "--timeout=40m")

	//run the command
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
}
