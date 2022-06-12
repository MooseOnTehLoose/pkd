package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

func generateMachineDeployment(cluster Cluster) {

	for nodesetName, nodes := range cluster.NodePools {

		md := MachineDeployment{}
		md.APIVersion = "cluster.x-k8s.io/v1beta1"
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
		md.Spec.Template.Spec.Version = "v1.22.8"

		data, err := yaml.Marshal(&md)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-"+nodesetName+"-MachineDeployment.yaml", data, 0644)
		if err != nil {
			log.Fatal(err)
		}

	}
}
