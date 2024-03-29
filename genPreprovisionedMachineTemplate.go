package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func generatePreprovisionedMachineTemplate(cluster pkdCluster) {
	for nodesetName, nodes := range cluster.NodePools {
		pmt := PreprovisionedMachineTemplate{}
		pmt.APIVersion = "infrastructure.cluster.konvoy.d2iq.io/v1alpha1"
		pmt.Kind = "PreprovisionedMachineTemplate"
		pmt.Metadata.Name = cluster.MetaData.Name + "-" + nodesetName
		pmt.Metadata.Namespace = "default"
		pmt.Spec.Template.Spec.InventoryRef.Name = cluster.MetaData.Name + "-" + nodesetName
		pmt.Spec.Template.Spec.InventoryRef.Namespace = "default"
		pmt.Spec.Template.Spec.OverrideRef.Name = cluster.MetaData.Name + "-" + nodesetName + "-override"

		genOverride2_6_0(pmt.Spec.Template.Spec.OverrideRef.Name, nodes, cluster.Registry, cluster.AirGap.Enabled)

		data, err := yaml.Marshal(&pmt)
		if err != nil {
			log.Fatal(err)
		}
		err = os.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-PreprovisionedMachineTemplate.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}

	}
}
