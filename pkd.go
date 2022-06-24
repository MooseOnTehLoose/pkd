package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
			fmt.Println("Generating cluster.yaml")
			initYaml()
		case arg1 == "airgap":
			fmt.Println("Downloading all Air Gap Resources")
			createAirGapBundle(loadCluster())
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
		case arg1 == "get-bundle":
			cluster := loadCluster()
			fmt.Printf("Cluster YAML loaded into PKD\n")
			createAirGapBundle(cluster)

		//no args or bad args
		default:
			fmt.Printf("Usage:\n" +
				" pkd 						prints usage\n" +
				" pkd init					create cluster.yaml and kommander.yaml templates\n" +
				" pkd up [yee-haw]			create all yaml resources needed to deploy a cluster, optional cowboy mode\n" +
				" pkd bootstrap [up/down]	control the bootstrap cluster\n" +
				" pkd version				grab the PKD, DKP and Kommander cli versions\n" +
				" pkd get-bundle			download an airgap bundle\n" +
				" pkd unzip-bundle			unpack an airgap bundle into the current directory\n\n")
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

	//people tend to forget to delete their bootstrap clusters
	//we should probably ask before deleting in a future release
	cluster := loadCluster()
	fmt.Printf("Cluster YAML loaded into PKD\n")

	//create inventory.yaml
	if cluster.AirGap.Enabled {
		generateInventory(cluster)
		copy(cluster.MetaData.SshPrivateKey, "kib/"+cluster.MetaData.SshPrivateKey)
		seedRegistry(cluster.Registry.Host, cluster.Registry.Password, cluster.Registry.Password)
		seedHosts(cluster.MetaData.DKPversion, cluster.AirGap.OsVersion)
		loadBootstrapImage(cluster.MetaData.DKPversion)
	}

	bootstrap("down")
	bootstrap("up")

	//This loads the user customizable values to generate a cluster from cluster.yaml

	//set defaults if not specified in cluster.yaml
	if cluster.MetaData.PodSubnet == "" {
		cluster.MetaData.PodSubnet = "192.168.0.0/16"
	}
	if cluster.MetaData.ServiceSubnet == "" {
		cluster.MetaData.ServiceSubnet = "10.96.0.0/12"
	}

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
	//this needs to be removed in a future version of PKD, once this bug is fixed in an official release
	if controlPlaneReplicas == "1" {
		controlPlaneReplicas = "3"
	}

	//Generate the cluster.yaml dry run output
	dkpDryRun(cluster.MetaData.Name, cluster.MetaData.KubeVipLoadbalancer, cluster.MetaData.InterfaceName, controlPlaneReplicas)
	fmt.Printf("Dry Run Completed\n")

	//Read in the Dry Run output and generate individual object file from it
	dryRunOutput, err := os.Open(cluster.MetaData.Name + ".yaml")
	if err != nil {
		panic(err)
	}
	dryRunDecoder := yaml.NewDecoder(dryRunOutput)

	// for each object in dry run, read it and convert to a yaml file
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

		file, err = yaml.Marshal(&spec)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile(fileName, file, 0644)
		if err != nil {
			log.Fatal(err)
		}

	}

	generateCapiCluster(cluster)
	generateCalicoConfigMap(cluster)
	generateKubeadmControlPlane(cluster)
	generateControlPlanePreprovisionedMachineTemplate(cluster)
	generatePreprovisionedMachineTemplate(cluster)
	generateKubeadmConfigTemplate(cluster)
	generateMachineDeployment(cluster)

	fmt.Printf("Generated all Custom Resources for NodePools\n")

	//before we apply resources check for the pause flag, ie ./pkd up yee-haw
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

	generateMlbConfigMap(cluster)
	fmt.Printf("Applied Metal-LB ConfigMap\n\n")

	if cluster.AirGap.Enabled {
		fmt.Println("The DKP cluster has now been deployed. You can proceed to deploying Kommander via:\n\n" +
			"./dkp install kommander --init --airgapped > install.yaml\n" +
			"./dkp install kommander --installer-config ./install.yaml" +
			"--kommander-applications-repository kommander-applications-" + cluster.MetaData.DKPversion + ".tar.gz" +
			"--charts-bundle dkp-kommander-charts-bundle-" + cluster.MetaData.DKPversion + ".tar.gz")
	} else {
		fmt.Println("The DKP cluster has now been deployed. You can proceed to deploying Kommander via:\n\n" +
			"./dkp install kommander --init > kommander.yaml\n" +
			"./dkp install kommander --installer-config kommander.yaml")
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

func loadCluster() pkdCluster {
	clusterYaml, err := ioutil.ReadFile("cluster.yaml")

	if err != nil {

		log.Fatal(err)
	}
	data := pkdCluster{
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

	exampleCluster := pkdCluster{
		MetaData:     MetaData{},
		Registry:     Registry{},
		Controlplane: NodePool{},
		NodePools:    map[string]NodePool{},
	}

	exampleCluster.MetaData.Name = "Demo Cluster"
	exampleCluster.MetaData.SshUser = "user"
	exampleCluster.MetaData.SshPrivateKey = "id_rsa"
	exampleCluster.MetaData.InterfaceName = "ens192"
	exampleCluster.MetaData.KubeVipLoadbalancer = "10.0.0.10"
	exampleCluster.MetaData.KIBTimeout = "40"
	exampleCluster.MetaData.PivotTimeout = "20"
	exampleCluster.MetaData.PodSubnet = "192.168.0.0/16"
	exampleCluster.MetaData.ServiceSubnet = "10.96.0.0/12"
	exampleCluster.MetaData.MetalAddressRange = "10.0.0.20-10.0.0.24"
	exampleCluster.AirGap.Enabled = false
	exampleCluster.Registry.Host = "registry-1.docker.io"
	exampleCluster.Registry.Username = "user"
	exampleCluster.Registry.Password = "pass"
	exampleCluster.Registry.Auth = ""
	exampleCluster.Registry.IdentityToken = ""
	exampleCluster.Controlplane.Hosts = map[string]string{
		"controlplane1": "10.0.0.11",
		"controlplane2": "10.0.0.12",
		"controlplane3": "10.0.0.13",
	}
	exampleCluster.Controlplane.Flags = map[string]bool{
		"registry": true,
	}
	exampleCluster.NodePools = map[string]NodePool{
		"md-0": {
			Hosts: map[string]string{
				"worker1": "10.0.0.14",
				"worker2": "10.0.0.15",
				"worker3": "10.0.0.16",
				"worker4": "10.0.0.17",
				"worker5": "10.0.0.18",
			},
			Flags: map[string]bool{
				"registry": true,
			},
		},
		"md-1": {
			Hosts: map[string]string{
				"worker1": "10.0.0.19",
				"worker2": "10.0.0.20",
			},
			Flags: map[string]bool{
				"registry": true,
			},
		},
	}

	file, err := yaml.Marshal(&exampleCluster)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("cluster.yaml", file, 0644)
	if err != nil {
		log.Fatal(err)
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

	// ./dkp create capi-components --kubeconfig ${CLUSTER_NAME}.conf
	cmd := exec.Command("./dkp", "create", "capi-components", "--kubeconfig", clusterName+".conf")

	//run the command
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	// ./dkp move capi-resources --to-kubeconfig ${CLUSTER_NAME}.conf
	cmd = exec.Command("./dkp", "move", "capi-resources", "--to-kubeconfig", clusterName+".conf")

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

func createAirGapBundle(cluster pkdCluster) {

	osversion := cluster.AirGap.OsVersion
	k8sversion := cluster.AirGap.K8sVersion
	dkpversion := cluster.MetaData.DKPversion
	err := os.MkdirAll("download/kib/artifacts/images", 0755)
	if err != nil {
		log.Fatal(err)
	}

	err = copy("pkd", "download/pkd")
	if err != nil {
		log.Fatal(err)
	}
	err = copy("cluster.yaml", "download/cluster.yaml")
	if err != nil {
		log.Fatal(err)
	}
	//download all resources for specific versions

	//unznip DKP
	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/dkp_"+dkpversion+"_linux_amd64.tar.gz", "download/dkp_"+dkpversion+"_linux_amd64.tar.gz")
	decompress("download/dkp_"+dkpversion+"_linux_amd64.tar.gz", "download")

	//unzip KIB  into kib dir
	downloadFile("https://github.com/mesosphere/konvoy-image-builder/releases/download/v1.12.0/konvoy-image-bundle-v1.12.0_linux_amd64.tar.gz", "download/konvoy-image-bundle-v1.12.0_linux_amd64.tar.gz")
	decompress("download/konvoy-image-bundle-v1.12.0_linux_amd64.tar.gz", "download/kib")

	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/konvoy-bootstrap_"+dkpversion+".tar", "download/konvoy-bootstrap_"+dkpversion+".tar")
	downloadFile("https://downloads.d2iq.com/dkp/airgapped/os-packages/"+k8sversion+"_"+osversion+".tar.gz", "download/kib/artifacts/"+k8sversion+"_"+osversion+".tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/airgapped/kubernetes-images/"+k8sversion+"_images.tar.gz", "download/kib/artifacts/images/"+k8sversion+"_images.tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/airgapped/pip-packages/pip-packages.tar.gz", "download/kib/artifacts/pip-packages.tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/konvoy_image_bundle_"+dkpversion+"_linux_amd64.tar.gz", "download/konvoy-image-bundle.tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/kommander-image-bundle-"+dkpversion+".tar.gz", "download/kommander-image-bundle.tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/dkp-kommander-charts-bundle-"+dkpversion+".tar.gz", "download/dkp-kommander-charts-bundle-"+dkpversion+".tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp"+dkpversion+"/kommander-applications-"+dkpversion+".tar.gz", "download/kommander-applications-"+dkpversion+".tar.gz")
	downloadFile("https://downloads.d2iq.com/dkp/"+dkpversion+"/dkp-insights-image-bundle-"+dkpversion+".tar.gz", "download/dkp-insights-image-bundle.tar.gz")

	//remove KIB tar fil
	//compress all resources into archive
	compress("./download", dkpversion)
	//delete remaining files
}

func downloadFile(url string, path string) {

	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Downloaded: " + url + "\n To: " + path)
}

//used to create the final AirGap Bundle
func compress(downloadpath string, version string) {
	var dirBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&dirBuffer)
	tarWriter := tar.NewWriter(gzipWriter)

	// walk through every file in the folder
	if err := filepath.Walk(downloadpath, func(path string, info fs.FileInfo, funcErr error) error {

		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(path)
		// write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		// if not a dir, write file content
		if !info.IsDir() {
			data, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tarWriter, data); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		fmt.Println(err)
	}

	if err := tarWriter.Close(); err != nil {
		fmt.Println(err)
	}
	if err := gzipWriter.Close(); err != nil {
		fmt.Println(err)
	}

	// write the .tar.gzip
	fileToWrite, err := os.OpenFile("./AirGapBundle-dkp"+version+".tar.gz", os.O_CREATE|os.O_RDWR, os.FileMode(0755))
	if err != nil {
		panic(err)
	}
	if _, err := io.Copy(fileToWrite, &dirBuffer); err != nil {
		panic(err)
	}

}

func decompress(src string, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		os.Exit(1)
	}
	gzf, err := gzip.NewReader(f)
	if err != nil {
		fmt.Println(err)
	}
	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if header.Typeflag == tar.TypeReg {
			targetFile := filepath.Join(dest, header.Name)
			cut := -1
			if runtime.GOOS == "windows" {
				cut = strings.LastIndex(targetFile, "\\")
			} else {
				cut = strings.LastIndex(targetFile, "/")
			}
			if cut == -1 {
				fmt.Println("Error: no path separator in filepath for: " + targetFile)
			} else {
				err = os.MkdirAll(targetFile[0:cut], 0755)
				if err != nil {
					fmt.Println(err)
				}
				if _, err := os.Stat("targetFile"); os.IsNotExist(err) {
					file, err := os.OpenFile(targetFile, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					if err != nil {
						return err
					}
					if _, err := io.Copy(file, tarReader); err != nil {
						return err
					}
				}
			}
		}

	}

	f.Close()
	return err
}

func copy(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		fmt.Printf("Failed to copy file: %s\n", err.Error())
	}
	return err
}

func seedRegistry(host string, user string, password string) {

	// ./dkp push image-bundle --image-bundle konvoy-image-bundle.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd := exec.Command("./dkp", "push", "image-bundle", "konvoy-image-bundle.tar.gz", "--to-registry", host, "--to-registry-username", user, "--to-registry-password", password)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	// ./dkp push image-bundle --image-bundle kommander-image-bundle.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd = exec.Command("./dkp", "push", "image-bundle", "kommander-image-bundle.tar.gz", "--to-registry", host, "--to-registry-username", user, "--to-registry-password", password)
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

	// ./dkp push image-bundle --image-bundle dkp-insights-image-bundle-v2.2.0.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd = exec.Command("./dkp", "push", "image-bundle", "dkp-insights-image-bundle.tar.gz", "--to-registry", host, "--to-registry-username", user, "--to-registry-password", password)
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
func seedHosts(dkpVersion string, bundleOs string) {

	// ./konvoy-image upload artifacts --container-images-dir=./artifacts/images/ --os-packages-bundle=./artifacts/"$VERSION"_"$BUNDLE_OS".tar.gz --pip-packages-bundle=./artifacts/pip-packages.tar.gz
	cmd := exec.Command("./kib/konvoy-image", "upload", "artifacts", "--container-images-dir=./kib/artifacts/images/",
		"--os-packages-bundle=./kib/artifacts/"+dkpVersion+"_"+bundleOs+".tar.gz", "--pip-packages-bundle=./kib/artifacts/pip-packages.tar.gz")
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
}
func loadBootstrapImage(version string) {
	// docker load -i konvoy-bootstrap_v2.2.0.tar
	cmd := exec.Command("docker", "load", "-i", "konvoy-bootstrap_v"+version+".tar")
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
