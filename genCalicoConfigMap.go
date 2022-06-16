package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

func generateCalicoConfigMap(cluster pkdCluster) {

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
		"      cidr: " + cluster.MetaData.PodSubnet + "\n" +
		"      encapsulation: IPIP\n" +
		"      natOutgoing: Enabled\n" +
		"      nodeSelector: all()\n" +
		"    bgp: Enabled\n"

	calicoCM := k8sObject{}
	calicoCM.APIVersion = "v1"
	calicoCM.Kind = "ConfigMap"
	calicoCM.Metadata = map[string]interface{}{"name": "calico-cni-" + cluster.MetaData.Name, "namespace": "default"}
	calicoCM.Data = map[string]interface{}{"custom-resources.yaml": customResources}
	file, err := yaml.Marshal(&calicoCM)
	if err != nil {
		log.Fatal(err)
	}
	//calico-cni-installation-${cluster_name}-ConfigMap.yaml
	err = ioutil.WriteFile("resources/calico-cni-installation-"+cluster.MetaData.Name+"-ConfigMap.yaml", file, 0644)
	if err != nil {
		log.Fatal(err)
	}

}
