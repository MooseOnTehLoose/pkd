package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const pkdVersion = "v0.2.0-2.2.0-beta1"

func main() {

	argNum := len(os.Args)
	// you must run pkd with at least one argument
	if argNum >= 2 {
		arg1 := os.Args[1]

		switch {
		//init
		case arg1 == "init":
			fmt.Printf("Generating cluster.yaml")
			initYaml()
		case arg1 == "airgap":
			fmt.Printf("Downloading and Seeding all Air Gap Resources")

		//up
		case arg1 == "up":
			if argNum >= 3 && os.Args[2] == "yee-haw" {
				fmt.Println("Good Luck Cowboy!")
				up("pause")
			} else {
				up("normal")
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
			if argNum == 3 {
				fmt.Printf("Merging " + os.Args[2] + " kubeconfig into ~/.kube/config\n")
				mergeKubeconfig(os.Args[2])
			} else {
				fmt.Println("Usage:\n  pkd merge <cluster-name>")
			}
		case arg1 == "version":

			fmt.Println("PKD Version: " + pkdVersion)
			//check if the dkp and kommander cli are present
			//kubectl --kubeconfig ${CLUSTER_NAME}.conf wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
			cmd := exec.Command("./dkp", "version")

			//run the command
			output, err := cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				log.Fatal(err)
			}
			//kubectl --kubeconfig ${CLUSTER_NAME}.conf wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
			cmd = exec.Command("./kommander", "version")

			//run the command
			output, err = cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				log.Fatal(err)
			}

		//no args or bad args
		default:
			fmt.Printf("Usage:\n" +
				" pkd 						prints usage\n" +
				" pkd init					create cluster.yaml and kommander.yaml templates\n" +
				" pkd up [yee-haw]			create all yaml resources needed to deploy a cluster, optional cowboy mode\n" +
				" pkd bootstrap [up/down]	control the bootstrap cluster\n" +
				" pkd version				grab the PKD, DKP and Kommander cli versions\n")
		}

	}
}

//Read in cluster.yaml and start the cluster creation process
func up(modifier string) {

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
	genCPPI(cluster.MetaData, cluster.Controlplane)
	fmt.Printf("Generated Control Plane PPI\n")

	//For Each NodePool, create a Preprovisioned Inventory Object
	//mdval sets the machinedeployment name ie md-0

	for nodesetName, nodes := range cluster.NodePools {
		genPPI(cluster.MetaData, nodes, nodesetName)
		fmt.Printf("Generated " + nodesetName + " PPI\n")

	}

	//apply the PreProvisionedInventory objects to the bootstrap cluster
	applyPPI(cluster.MetaData.Name)
	fmt.Printf("Applied all PPI\n")

	controlPlaneReplicas := strconv.Itoa(len(cluster.Controlplane.Hosts))

	//there is a bug in dkp 2.2.X, you must use 3 control planes then fix the object after dry run
	if controlPlaneReplicas == "1" {
		controlPlaneReplicas = "3"
	}

	//Generate the cluster.yaml dry run output
	dkpDryRun(cluster.MetaData.Name, cluster.MetaData.Loadbalancer, cluster.MetaData.InterfaceName, controlPlaneReplicas)
	fmt.Printf("Dry Run Completed\n")

	podSubnet := cluster.MetaData.PodSubnet
	serviceSubnet := cluster.MetaData.ServiceSubnet

	//set defaults if not specified in cluster.yaml
	if podSubnet == "" {
		podSubnet = "192.168.0.0/16"
	}
	if serviceSubnet == "" {
		serviceSubnet = "10.96.0.0/12"
	}

	//Read in the Dry Run output and generate individual object file from it
	dryRunOutput, err := os.Open(cluster.MetaData.Name + ".yaml")
	if err != nil {
		panic(err)
	}
	dryRunDecoder := yaml.NewDecoder(dryRunOutput)

	// for each object in dry run, read it and convert to single file
	for {
		spec := new(k8sObject)
		err := dryRunDecoder.Decode(&spec)
		if spec == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			panic(err)
		}
		resourceName := spec.Metadata["name"].(string)
		resourceKind := spec.Kind
		fileName := "resources/" + resourceName + "-" + resourceKind + ".yaml"
		var file []byte

		if resourceName == cluster.MetaData.Name && resourceKind == "Cluster" {
			test := k8sCluster{
				APIVersion: "cluster.x-k8s.io/v1alpha4",
				Kind:       resourceKind,
				MetaData: k8sClusterMetadata{
					Labels: map[string]string{
						"konvoy.d2iq.io/cluster-name": cluster.MetaData.Name,
						"konvoy.d2iq.io/cni":          "calico",
						"konvoy.d2iq.io/csi":          "local-volume-provisioner",
						"konvoy.d2iq.io/osHint":       "",
						"konvoy.d2iq.io/provider":     "preprovisioned",
					},
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: k8sClusterSpec{
					ClusterNetwork: k8sClusterClusterNetwork{
						Pods: k8sPods{
							CidrBlocks: []string{podSubnet},
						},
						Services: k8sServices{
							CidrBlocks: []string{serviceSubnet},
						},
					},
					ControlPlaneEndpoint: k8sControlPlaneEndpoint{
						Host: "",
						Port: 0,
					},
					ControlPlaneRef: k8sControlPlaneRef{
						APIVersion: "controlplane.cluster.x-k8s.io/v1alpha4",
						Kind:       "KubeadmControlPlane",
						Name:       cluster.MetaData.Name + "-control-plane",
						Namespace:  "default",
					},
					InfrastructureRef: k8sInfrastructureRef{
						APIVersion: "infrastructure.cluster.konvoy.d2iq.io/v1alpha1",
						Kind:       "PreprovisionedCluster",
						Name:       cluster.MetaData.Name,
						Namespace:  "default",
					},
				},
			}
			file, err = yaml.Marshal(&test)

		} else if resourceName == "calico-cni-"+cluster.MetaData.Name && resourceKind == "ConfigMap" {

			customResources := "apiVersion: operator.tigera.io/v1\n" +
				"kind: Installation\n" +
				"metadata:\n" +
				"  name: default\n" +
				"spec:\n" +
				"  # Configures Calico networking.\n" +
				"  calicoNetwork:\n" +
				"    # Note: The ipPools section cannot be modified post-install.\n" +
				"    ipPools:\n" +
				"    - blockSize: 26\n" +
				"      cidr: " + podSubnet + "\n" +
				"      encapsulation: IPIP\n" +
				"      natOutgoing: Enabled\n" +
				"      nodeSelector: all()\n" +
				"    bgp: Enabled\n"

			test := k8sObject{}
			test.APIVersion = "v1"
			test.Kind = "ConfigMap"
			test.Metadata = map[string]interface{}{"name": "calico-cni-" + cluster.MetaData.Name, "namespace": "default"}
			test.Data = map[string]interface{}{"custom-resources.yaml": customResources}

			file, err = yaml.Marshal(&test)

		} else if !(strings.Contains(resourceName, "md-0") ||
			(resourceName == cluster.MetaData.Name+"-control-plane" && resourceKind == "PreprovisionedMachineTemplate") ||
			(resourceName == cluster.MetaData.Name+"-control-plane" && resourceKind == "KubeadmControlPlane")) {
			file, err = yaml.Marshal(&spec)

		}
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile(fileName, file, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	generateKubeadmControlPlane(cluster)

	//If anything sets a flag to true, generate an override for it
	flagEnabled := generateControlPlanePreprovisionedMachineTemplate(cluster)
	flagEnabled = generatePreprovisionedMachineTemplate(cluster, flagEnabled)
	generateKubeadmConfigTemplate(cluster)
	generateMachineDeployment(cluster)
	fmt.Printf("Generated all Custom Resources for NodePools\n")

	genOverride(cluster.MetaData.Name, cluster.Registry, flagEnabled)
	fmt.Printf("Generated All Overrides\n")

	//before we apply resources check for the pause flag
	if modifier == "pause" {
		r := bufio.NewReader(os.Stdin)
		fmt.Println("Pausing, you can now manually edit objects in /resources before cluster creation")
		input := true
		for input {
			fmt.Printf("Ready to continue? Type y or yes to confirm: ")

			res, err := r.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}

			// Empty input (i.e. "\n")
			if len(res) < 2 {
				input = true
			} else if strings.ToLower(strings.TrimSpace(res)) == "yes" || strings.ToLower(strings.TrimSpace(res)) == "y" {
				input = false
			}

		}

	}

	//delete the Dry Run cluster YAML after we're done with it
	//Moved till after the pause window in case you need to check it
	err = os.Remove(cluster.MetaData.Name + ".yaml")
	if err != nil {
		log.Fatal(err)
	}

	applyResources(cluster.MetaData.Name)
	fmt.Printf("Applied All Resources, Cluster Spinning Up\n")

	timeout := "40"
	if cluster.MetaData.KIBTimeout != "" {
		timeout = cluster.MetaData.KIBTimeout
	}
	waitForClusterReady(cluster.MetaData.Name, timeout)
	fmt.Printf("Cluster Is Ready\n")

	getKubeconfig(cluster.MetaData.Name)
	fmt.Printf("Grabbed the Kubeconfig\n")

	if cluster.MetaData.PivotTimeout != "" {
		timeout = cluster.MetaData.PivotTimeout
	} else {
		timeout = "20"
	}
	pivotCluster(cluster.MetaData.Name, timeout)
	fmt.Printf("Pivoted the Cluster\n")

	bootstrap("down")
	fmt.Printf("Cleaned up the Bootstrap Cluster\n")

	mergeKubeconfig(cluster.MetaData.Name)
	fmt.Printf("Merged the Kubeconfig\n")
}

func genOverride(clusterName string, registryInfo Registry, flags map[string]bool) {

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

func genCPPI(mdata MetaData, cplane NodePool) {

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
					"name":      mdata.Name + "-ssh-key",
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

func genPPI(mdata MetaData, npool NodePool, nodesetName string) {

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
			"name":      mdata.Name + "-" + nodesetName,
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
					"name":      mdata.Name + "-ssh-key",
					"namespace": "default",
				},
			},
		},
	}

	data, err := yaml.Marshal(&ppi)

	if err != nil {
		log.Fatal(err)
	}

	err2 := ioutil.WriteFile("resources/"+mdata.Name+"-"+nodesetName+"-PreprovisionedInventory.yaml", data, 0644)

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
		Controlplane: NodePool{},
		NodePools:    map[string]NodePool{},
	}

	err2 := yaml.Unmarshal(clusterYaml, &data)

	if err2 != nil {

		log.Fatal(err2)
	}

	return data
}

func initYaml() {
	initcluster := Cluster{
		MetaData: MetaData{
			Name:          "cluster-name",
			SshUser:       "user",
			SshPrivateKey: "id_rsa",
			InterfaceName: "ens192",
			Loadbalancer:  "10.0.0.10",
			PodSubnet:     "192.168.0.0/16",
			ServiceSubnet: "10.96.0.0/12",
		},
		Registry: Registry{
			Host:          "registry-1.docker.io",
			Username:      "",
			Password:      "",
			Auth:          "",
			IdentityToken: "",
		},
		Controlplane: NodePool{
			Hosts: map[string]string{"controlplane1": "10.0.0.11", "controlplane2": "10.0.0.12", "controlplane3": "10.0.0.13"},
			Flags: map[string]bool{"registry": true},
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

	mergedConfigs := "./" + clusterName + ".conf:" + os.Getenv("HOME") + "/.kube/config"
	fmt.Println("Setting Environment Variable for kubeconfig to: \n  " + mergedConfigs)
	os.Setenv("KUBECONFIG", mergedConfigs)

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
	fmt.Println("Switching to Context: " + context)

	cmd3 := exec.Command("kubectl", "config", "use-context", context)
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

	var errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(errb.String())
}

func createSSHSecret(clusterName string, keyName string) {

	//exec.Command("kubectl create secret generic " + cluster.MetaData.Name + "-ssh-key --from-file=ssh-privatekey=" + cluster.MetaData.SshPrivateKey)
	//exec.Command("kubectl label secret " + cluster.MetaData.Name + "-ssh-key clusterctl.cluster.x-k8s.io/move=")

	cmd := exec.Command("kubectl", "create", "secret", "generic", clusterName+"-ssh-key", "--from-file=ssh-privatekey="+keyName)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("kubectl", "label", "secret", clusterName+"-ssh-key", "clusterctl.cluster.x-k8s.io/move=")
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
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

func dkpDryRun(clusterName string, clusterLoadBalancer string, interfaceName string, controlPlaneReplicas string) {

	cmd := exec.Command(
		"./dkp", "create", "cluster", "preprovisioned",
		"--cluster-name", clusterName,
		"--control-plane-endpoint-host", clusterLoadBalancer,
		"--control-plane-replicas", controlPlaneReplicas,
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
			//kubectl create -f <cluster-name>-PreProvisionedInventory.yaml
			//changed from apply to create because tigera throws an error via apply, too big
			cmd := exec.Command("kubectl", "create", "-f", path)
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

func waitForClusterReady(clusterName string, kibTimeout string) {

	//kubectl  wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
	cmd := exec.Command("kubectl", "wait", "--for=condition=Ready", "clusters/"+clusterName, "--timeout="+kibTimeout+"m")

	//run the command
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
	//give the user time to fix any machines stuck in pending

	fmt.Printf("Waiting up to 1 hour for all machines to be ready\nTo check if your machines are stuck, use command:\n\n  kubectl get machines\n\n")
	cmd = exec.Command("kubectl", "wait", "--for=condition=Ready", "machine", "--all", "--timeout=60m")

	//run the command
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
}

func pivotCluster(clusterName string, pivotTimeout string) {
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
	cmd = exec.Command("kubectl", "--kubeconfig", clusterName+".conf", "wait", "--for=condition=ControlPlaneReady", "clusters/"+clusterName, "--timeout="+pivotTimeout+"m")

	//run the command
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	//kubectl --kubeconfig ${CLUSTER_NAME}.conf wait --for=condition=Ready "cluster/${CLUSTER_NAME}" --timeout=40m
	cmd = exec.Command("kubectl", "--kubeconfig", clusterName+".conf", "wait", "--for=condition=Ready", "clusters/"+clusterName, "--timeout=40m")

	//run the command
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
}
