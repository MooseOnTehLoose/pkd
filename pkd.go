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
	registry := false
	gpu := false
	gpuRegistry := false

	fmt.Printf("Deploying DKP Cluster\n")
	//bootstrap("down")
	//bootstrap("up")

	cluster := loadCluster()

	//exec.Command("kubectl create secret generic " + clusterName + "-ssh-key --from-file=ssh-privatekey=" + sshKey)
	//exec.Command("kubectl label secret " + clusterName + "-ssh-key clusterctl.cluster.x-k8s.io/move=")

	//Create a ControlPlane PreProvisionedInventory Ojbect
	genCPPI(cluster.MetaData, "controlplane", cluster.Controlplane)
	if cluster.Controlplane.Flags["registry"] {
		registry = true
		fmt.Printf("Enabling Registry override for Control Plane\n")
	}
	if cluster.Controlplane.Flags["gpu"] {
		gpu = true
		fmt.Printf("Enabling GPU override for Control Plane\n")

	}
	if registry && gpu {
		gpuRegistry = true
	}

	//For Each NodePool, create a Preprovisioned Inventory Object
	//mdval sets the machinedeployment name ie md-0
	mdVal := 0

	for nodesetName, nodes := range cluster.NodePools {
		genPPI(cluster.MetaData, nodesetName, nodes, mdVal)

		if nodes.Flags["registry"] {
			registry = true
			fmt.Printf("Enabling Registry override for NodePool md-" + strconv.Itoa(mdVal) + "\n")
		}
		if nodes.Flags["gpu"] {
			gpu = true
			fmt.Printf("Enabling GPU override for NodePool md-" + strconv.Itoa(mdVal) + "\n")

		}
		if registry && gpu {
			gpuRegistry = true
		}
		mdVal++
	}

	//todo: write function that applies all ppi after creation
	//kubectl apply -f preprovisioned_inventory.yaml
	genOverride(cluster.Registry)

}

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

//func createOverrideSecret(name string, override string) {
//
//	exec.Command("kubectl", "create secret generic "+name+"-"+override+"-override --from-file="+override+".yaml="+override+".yaml")
//	exec.Command("kubectl", "label secret "+name+"-"+override+"-override clusterctl.cluster.x-k8s.io/move=")
//}
