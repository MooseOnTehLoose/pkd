package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func generateInventory(cluster pkdCluster) {

	clusterInventory := Inventory{
		All: struct {
			Vars struct {
				AnsibleUser              string "yaml:\"ansible_user\""
				AnsiblePort              int    "yaml:\"ansible_port\""
				AnsibleSSHPrivateKeyFile string "yaml:\"ansible_ssh_private_key_file\""
			} "yaml:\"vars\""
			Hosts map[string]AnsibleHost "yaml:\"hosts\""
		}{},
	}

	clusterInventory.All.Hosts = map[string]AnsibleHost{}
	clusterInventory.All.Vars.AnsibleUser = cluster.MetaData.SshUser
	clusterInventory.All.Vars.AnsiblePort = 22
	clusterInventory.All.Vars.AnsibleSSHPrivateKeyFile = cluster.MetaData.SshPrivateKey

	for _, ip := range cluster.Controlplane.Hosts {
		node := AnsibleHost{}
		node.AnsibleHost = ip
		clusterInventory.All.Hosts[ip] = node

	}
	for _, npool := range cluster.NodePools {
		for _, ip := range npool.Hosts {
			node := AnsibleHost{}
			node.AnsibleHost = ip
			clusterInventory.All.Hosts[ip] = node
		}

	}

	file, err := yaml.Marshal(&clusterInventory)
	if err != nil {
		log.Fatal(err)
	}
	os.MkdirAll("kib", os.ModePerm)
	err = os.WriteFile("kib/inventory.yaml", file, 0644)
	if err != nil {
		log.Fatal(err)
	}

}
