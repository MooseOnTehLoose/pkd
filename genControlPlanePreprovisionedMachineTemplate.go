package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

func generateControlPlanePreprovisionedMachineTemplate(cluster pkdCluster) map[string]bool {

	flagEnabled := map[string]bool{"registry": false, "gpu": false, "registryGPU": false}
	nodesetName := "control-plane"
	nodes := cluster.Controlplane
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
		//pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
		flagEnabled["registryGPU"] = true
	case nodes.Flags["registry"]:
		pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-registry-override"
		//pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
		flagEnabled["registry"] = true
	case nodes.Flags["gpu"]:
		pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-gpu-override"
		//pmt.Spec.Template.Spec.OverrideRef.Namespace = "default"
		flagEnabled["gpu"] = true
	}

	data, err := yaml.Marshal(&pmt)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-PreprovisionedMachineTemplate.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	return flagEnabled
}
