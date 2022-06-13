package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"

	"gopkg.in/yaml.v3"
)

func generateMlbConfigMap(cluster Cluster) {

	mlb := mlbConfigMap{}
	mlb.APIVersion = "v1"
	mlb.Kind = "ConfigMap"
	mlb.Metadata.Name = "config"
	mlb.Metadata.Namespace = "metallb-system"
	mlb.Data.Config =
		"address-pools:\n" +
			"- name: default\n" +
			"  protocol: layer2\n" +
			"  addresses:\n" +
			"  - " + cluster.MetaData.MetalLBAddresses

	data, err := yaml.Marshal(&mlb)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-Metal-LB-ConfigMap.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	//kubectl create -f <cluster-name>-PreProvisionedInventory.yaml
	//changed from apply to create because tigera throws an error via apply, too big
	cmd := exec.Command("kubectl", "create", "-f", "resources/"+cluster.MetaData.Name+"-Metal-LB-ConfigMap.yaml")
	//run the command
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
