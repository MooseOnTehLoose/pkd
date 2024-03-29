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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/schollz/progressbar/v3"
	"gopkg.in/yaml.v3"
)

const pkdVersion = "v1.0.3-dkp2.6.0"
const bundleDir = "dkp-air-gapped-bundle_v2.6.0_linux_amd64/dkp-v2.6.0/"

func main() {

	argNum := len(os.Args)
	// you must run pkd with at least one argument
	if argNum >= 2 {
		arg1 := os.Args[1]

		switch {
		//init
		case arg1 == "init":
			if argNum >= 3 && os.Args[2] == "ag" {
				fmt.Println("Generating air gap cluster.yaml")
				initAGYaml()
			} else {
				fmt.Println("Generating cluster.yaml")
				initYaml()
			}
		//up
		case arg1 == "up":
			if argNum >= 3 && os.Args[2] == "yee-haw" {
				fmt.Println("Good Luck Cowboy!")
				up("pause")
			} else {
				up("normal")
			}
		case arg1 == "version":

			fmt.Println("PKD Version: " + pkdVersion)
			//check if the dkp cli is present
			cmd := exec.Command("./dkp", "version")

			//run the command
			output, err := cmd.CombinedOutput()
			fmt.Println(string(output))
			if err != nil {
				log.Fatal(err)
			}
		//no args or bad args
		default:
			fmt.Printf("Usage:\n" +
				" pkd init [ag]				create cluster.yaml for on prem or air gap\n" +
				" pkd airgap				download all airgap resources and create a tar.gz bundle\n" +
				" pkd up [yee-haw]			create all yaml resources needed to deploy a cluster, optional cowboy mode\n" +
				" pkd version				grab the PKD, DKP and Kommander cli versions\n")
		}

	}
}

// Read in cluster.yaml and start the cluster creation process
func up(modifier string) {

	//We need to generate the folder to store our k8s objects after creation
	os.MkdirAll("resources", os.ModePerm)
	fmt.Printf("Created resources directory\n")
	//Overrides allow us to set docker hub credentials
	os.MkdirAll("overrides", os.ModePerm)
	fmt.Printf("Created overrides directory\n")
	//we store DKP binaries here
	os.MkdirAll("dkpBinaries", os.ModePerm)
	fmt.Printf("Created dkp storage directory\n")

	//This loads the user customizable values to generate a cluster from cluster.yaml
	cluster := loadCluster()
	fmt.Printf("Cluster YAML loaded into PKD\n")

	//check if dkp version is present
	if _, err := os.Stat("dkp"); err == nil {
		//get the version of DKP and compare to cluster info
		cmd := exec.Command("./dkp", "version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatal(err)
		}
		lines := bytes.Split(output, []byte("\n"))
		version := string(lines[1][5:])
		if version == cluster.MetaData.DKPversion {
			fmt.Println("DKP Version " + version + " detected! Continuing.")
		} else {
			fmt.Println("DKP Binary: " + version + "does not match configured version: " + cluster.MetaData.DKPversion + " ! Exiting!")
			return
		}
	} else {
		fmt.Println("DKP Binary not present! Exiting!")
		return
	}

	//create inventory.yaml for airgap clusters
	//we no longer use a separate kib as of DKP 2.4.0, it is part of the "everything" airgap bundle
	if cluster.AirGap.Enabled {

		//verify presence of airgap bundle
		// dkp-air-gapped-bundle_v2.6.0_linux_amd64/dkp-v2.6.0/kib
		fmt.Println("Please ensure DKP Airgap Bundle is unzipped to current directory")

		if stat, err := os.Stat(bundleDir); err == nil && stat.IsDir() {
			// path is a directory
		} else {
			fmt.Println("Could not detect Air Gap Bundle")
			return
		}

		generateInventory(cluster)
		fmt.Println("Ensure AirGap Bundle is in current directory before proceeding")
		fmt.Println("Copying ssh key defined in cluster.yaml to kib directory")
		copy(cluster.MetaData.SshPrivateKey, bundleDir+"kib/"+cluster.MetaData.SshPrivateKey)
		seedRegistry(cluster.Registry.Host, cluster.Registry.Username, cluster.Registry.Password, cluster.MetaData.DKPversion)
		seedHosts(cluster.AirGap.K8sVersion, cluster.AirGap.OsVersion, cluster.AirGap.ContainerdVersion, cluster.MetaData.DKPversion)
		loadBootstrapImage(cluster.MetaData.DKPversion)
	}

	bootstrap("down")
	bootstrap("up")

	//set defaults if not specified in cluster.yaml
	//ensure that these subnets don't collide with metal-lb!
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

	//Generate the cluster.yaml dry run output
	dkpDryRun(cluster.MetaData.Name, cluster.MetaData.KubeVipLoadbalancer, cluster.MetaData.InterfaceName, controlPlaneReplicas)
	fmt.Printf("Dry Run Completed, Converting to Individual Objects\n")

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
		err = os.WriteFile(fileName, file, 0644)
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
			"./dkp install kommander --installer-config install.yaml" +
			" --kommander-applications-repository kommander-applications-" + cluster.MetaData.DKPversion + ".tar.gz" +
			" --charts-bundle dkp-kommander-charts-bundle-" + cluster.MetaData.DKPversion + ".tar.gz")
	} else {
		fmt.Println("The DKP cluster has now been deployed. You can proceed to deploying Kommander via:\n\n" +
			"./dkp install kommander --init > kommander.yaml\n" +
			"./dkp install kommander --installer-config kommander.yaml")
	}
}

func loadCluster() pkdCluster {
	clusterYaml, err := os.ReadFile("cluster.yaml")

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
	exampleCluster.MetaData.DKPversion = "v2.6.0"
	exampleCluster.MetaData.Name = "demo-cluster"
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
	exampleCluster.Registry.Host = "https://registry-1.docker.io"
	exampleCluster.Registry.Username = "user"
	exampleCluster.Registry.Password = "pass"
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
	err = os.WriteFile("cluster.yaml", file, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func initAGYaml() {

	exampleCluster := pkdCluster{
		MetaData:     MetaData{},
		Registry:     Registry{},
		Controlplane: NodePool{},
		NodePools:    map[string]NodePool{},
	}
	exampleCluster.MetaData.DKPversion = "v2.6.0"
	exampleCluster.MetaData.Name = "demo-cluster"
	exampleCluster.MetaData.SshUser = "user"
	exampleCluster.MetaData.SshPrivateKey = "id_rsa"
	exampleCluster.MetaData.InterfaceName = "ens192"
	exampleCluster.MetaData.KubeVipLoadbalancer = "10.0.0.10"
	exampleCluster.MetaData.KIBTimeout = "40"
	exampleCluster.MetaData.PivotTimeout = "20"
	exampleCluster.MetaData.PodSubnet = "192.168.0.0/16"
	exampleCluster.MetaData.ServiceSubnet = "10.96.0.0/12"
	exampleCluster.MetaData.MetalAddressRange = "10.0.0.20-10.0.0.24"
	exampleCluster.AirGap.Enabled = true
	exampleCluster.AirGap.K8sVersion = "1.26.6"
	exampleCluster.AirGap.OsVersion = "centos_7_x86_64"
	exampleCluster.AirGap.ContainerdVersion = "centos-7.9-x86_64"
	exampleCluster.AirGap.IncludePKD = true
	exampleCluster.AirGap.PKDoS = "linux"
	exampleCluster.Registry.Host = "https://registry-1.docker.io"
	exampleCluster.Registry.Username = "user"
	exampleCluster.Registry.Password = "pass"
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
	}

	file, err := yaml.Marshal(&exampleCluster)
	if err != nil {
		log.Fatal(err)
	}
	err = os.WriteFile("cluster.yaml", file, 0644)
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

	fmt.Printf("Waiting up to 1 hour for all machines to be ready\nTo check if your machines are stuck, use command:\n\n  kubectl get job,pod,machines\n\n")
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

// used to create the final AirGap Bundle
func compress(downloadpath string, version string) {

	var dirBuffer bytes.Buffer

	gzipWriter := gzip.NewWriter(&dirBuffer)
	tarWriter := tar.NewWriter(gzipWriter)

	tarBar := progressbar.DefaultBytes(
		-1,
		"Building Tar Bundle",
	)

	// walk through every file in the folder
	if err := filepath.Walk(downloadpath, func(path string, info fs.FileInfo, funcErr error) error {

		//This header represents either a file or directory
		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(path)

		// write header to the tar file
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// if not a dir, write file content to the tar object
		if !info.IsDir() {

			data, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(io.MultiWriter(tarWriter, tarBar), data); err != nil {
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
	fileToWrite, err := os.OpenFile("./AirGapBundle-dkp-"+version+".tar.gz", os.O_CREATE|os.O_RDWR, os.FileMode(0755))
	if err != nil {
		panic(err)
	}

	// Dump the byte buffer to a tar.gz file
	if _, err := io.Copy(fileToWrite, &dirBuffer); err != nil {
		panic(err)
	}

	fmt.Printf("\n\nAirGap Bundle now available: AirGapBundle-dkp-" + version + ".tar.gz\n\n")

}

func decompress(src string, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		os.Exit(1)
	}

	var tarReader *tar.Reader

	if strings.Contains(src, "tar.gz") {
		gzf, err := gzip.NewReader(f)
		if err != nil {
			fmt.Println(err)
		}
		tarReader = tar.NewReader(gzf)

	} else if strings.Contains(src, ".tar") {
		tarReader = tar.NewReader(f)
	} else {
		fmt.Println("Error: File is not in a compatible archive format")
		fmt.Println("Must be either tar or gzip archive")

		return err
	}

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

func seedRegistry(host string, user string, password string, version string) {

	registryURL := strings.ReplaceAll(host, "https://", "")
	registryURL = strings.ReplaceAll(registryURL, "http://", "")

	//dkp-air-gapped-bundle_v2.6.0_linux_amd64/dkp-v2.6.0/container-images

	fmt.Println("Pushing Konvoy Image Bundle to Registry")
	// ./dkp push image-bundle --image-bundle konvoy-image-bundle.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd := exec.Command("./dkp", "push", "image-bundle", "--image-bundle", bundleDir+"container-images/konvoy-image-bundle.tar.gz", "--to-registry", registryURL, "--to-registry-username", user, "--to-registry-password", password)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pushing Kommander Image Bundle to Registry")
	// ./dkp push image-bundle --image-bundle "kommander-image-bundle.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd = exec.Command("./dkp", "push", "image-bundle", "--image-bundle", bundleDir+"container-images/kommander-image-bundle-"+version+".tar.gz", "--to-registry", registryURL, "--to-registry-username", user, "--to-registry-password", password)
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pushing DKP Insights Image Bundle to Registry")
	// ./dkp push image-bundle --image-bundle dkp-insights-image-bundle-v2.2.0.tar.gz --to-registry $DOCKER_REGISTRY_ADDRESS --to-registry-username testuser --to-registry-password
	cmd = exec.Command("./dkp", "push", "image-bundle", "--image-bundle", bundleDir+"container-images/dkp-insights-image-bundle-"+version+".tar.gz", "--to-registry", registryURL, "--to-registry-username", user, "--to-registry-password", password)
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}

func seedHosts(osVersion string, bundleOs string, cdVersion string, dkpVersion string) {

	fmt.Println("Using Konvoy Image Builder to upload artifacts to hosts")

	//	konvoy-image upload artifacts --container-images-dir=./artifacts/images/ \
	//	--os-packages-bundle=./artifacts/"$VERSION"_"$BUNDLE_OS".tar.gz \
	//	--pip-packages-bundle=./artifacts/pip-packages.tar.gz \
	//	--containerd-bundle=artifacts/containerd-1.4.13-d2iq.1-"$CONTAINERD_OS".tar.gz
	cmd := exec.Command("./konvoy-image", "upload", "artifacts", "--container-images-dir=artifacts/images/",
		"--os-packages-bundle=artifacts/"+osVersion+"_"+bundleOs+".tar.gz",
		"--pip-packages-bundle=artifacts/pip-packages.tar.gz",
		"--containerd-bundle=artifacts/containerd-1.4.13-d2iq.1-"+cdVersion+".tar.gz")
	cmd.Dir = (bundleDir + "/kib")
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
func loadBootstrapImage(version string) {
	fmt.Println("Loading the konvoy bootstrap docker image from file")
	// docker load -i konvoy-bootstrap-image-v2.6.0.tar
	cmd := exec.Command("docker", "load", "-i", bundleDir+"konvoy-bootstrap_image-"+version+".tar")
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
