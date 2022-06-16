package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

func generateCapiCluster(cluster pkdCluster) {

	capppCluster := capiCluster{
		APIVersion: "",
		Kind:       "",
		Metadata: struct {
			Labels struct {
				KonvoyD2IqIoClusterName  string "yaml:\"konvoy.d2iq.io/cluster-name\""
				KonvoyD2IqIoCni          string "yaml:\"konvoy.d2iq.io/cni\""
				KonvoyD2IqIoCsi          string "yaml:\"konvoy.d2iq.io/csi\""
				KonvoyD2IqIoLoadbalancer string "yaml:\"konvoy.d2iq.io/loadbalancer\""
				KonvoyD2IqIoOsHint       string "yaml:\"konvoy.d2iq.io/osHint\""
				KonvoyD2IqIoProvider     string "yaml:\"konvoy.d2iq.io/provider\""
			} "yaml:\"labels\""
			Name      string "yaml:\"name\""
			Namespace string "yaml:\"namespace\""
		}{},
		Spec: struct {
			ClusterNetwork struct {
				Pods struct {
					CidrBlocks []string "yaml:\"cidrBlocks\""
				} "yaml:\"pods\""
				Services struct {
					CidrBlocks []string "yaml:\"cidrBlocks\""
				} "yaml:\"services\""
			} "yaml:\"clusterNetwork\""
			ControlPlaneEndpoint struct {
				Host string "yaml:\"host\""
				Port int    "yaml:\"port\""
			} "yaml:\"controlPlaneEndpoint\""
			ControlPlaneRef struct {
				APIVersion string "yaml:\"apiVersion\""
				Kind       string "yaml:\"kind\""
				Name       string "yaml:\"name\""
				Namespace  string "yaml:\"namespace\""
			} "yaml:\"controlPlaneRef\""
			InfrastructureRef struct {
				APIVersion string "yaml:\"apiVersion\""
				Kind       string "yaml:\"kind\""
				Name       string "yaml:\"name\""
				Namespace  string "yaml:\"namespace\""
			} "yaml:\"infrastructureRef\""
		}{},
	}

	capppCluster.APIVersion = "cluster.x-k8s.io/v1beta1"
	capppCluster.Kind = "Cluster"
	capppCluster.Metadata.Labels.KonvoyD2IqIoClusterName = cluster.MetaData.Name
	capppCluster.Metadata.Labels.KonvoyD2IqIoCni = "calico"
	capppCluster.Metadata.Labels.KonvoyD2IqIoCsi = "local-volume-provisioner"
	capppCluster.Metadata.Labels.KonvoyD2IqIoLoadbalancer = "metallb"
	capppCluster.Metadata.Labels.KonvoyD2IqIoOsHint = ""
	capppCluster.Metadata.Labels.KonvoyD2IqIoProvider = "preprovisioned"
	capppCluster.Metadata.Name = cluster.MetaData.Name
	capppCluster.Metadata.Namespace = "default"
	capppCluster.Spec.ClusterNetwork.Pods.CidrBlocks = append(capppCluster.Spec.ClusterNetwork.Pods.CidrBlocks, cluster.MetaData.PodSubnet)
	capppCluster.Spec.ClusterNetwork.Services.CidrBlocks = append(capppCluster.Spec.ClusterNetwork.Services.CidrBlocks, cluster.MetaData.ServiceSubnet)
	capppCluster.Spec.ControlPlaneEndpoint.Host = ""
	capppCluster.Spec.ControlPlaneEndpoint.Port = 0
	capppCluster.Spec.ControlPlaneRef.APIVersion = "controlplane.cluster.x-k8s.io/v1beta1"
	capppCluster.Spec.ControlPlaneRef.Kind = "KubeadmControlPlane"
	capppCluster.Spec.ControlPlaneRef.Name = cluster.MetaData.Name + "-control-plane"
	capppCluster.Spec.ControlPlaneRef.Namespace = "default"
	capppCluster.Spec.InfrastructureRef.APIVersion = "infrastructure.cluster.konvoy.d2iq.io/v1alpha1"
	capppCluster.Spec.InfrastructureRef.Kind = "PreprovisionedCluster"
	capppCluster.Spec.InfrastructureRef.Name = cluster.MetaData.Name
	capppCluster.Spec.InfrastructureRef.Namespace = "default"

	file, err := yaml.Marshal(&capppCluster)
	if err != nil {
		log.Fatal(err)
	}
	//${CLUSTER_NAME}-Cluster.yaml
	err = ioutil.WriteFile("resources/"+cluster.MetaData.Name+"-Cluster.yaml", file, 0644)
	if err != nil {
		log.Fatal(err)
	}

}
