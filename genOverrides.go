package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"

	"gopkg.in/yaml.v3"
)

func genOverride(name string, nodes NodePool, registryInfo Registry, airgap bool) {

	override := kibOverride{}

	//os_packages_local_bundle_file: "{{ playbook_dir }}/../artifacts/{{ kubernetes_version }}_{{ ansible_distribution|lower }}_{{ ansible_distribution_major_version }}_x86_64.tar.gz"
	//pip_packages_local_bundle_file: "{{ playbook_dir }}/../artifacts/pip-packages.tar.gz"
	//images_local_bundle_dir: "{{ playbook_dir}}/../artifacts/images"

	if airgap {
		override.OsPackagesLocalBundleFile = "{{ playbook_dir }}/../artifacts/{{ kubernetes_version }}_{{ ansible_distribution|lower }}_{{ ansible_distribution_major_version }}_x86_64.tar.gz"
		override.PipPackagesLocalBundleFile = "{{ playbook_dir }}/../artifacts/pip-packages.tar.gz"
		override.ImagesLocalBundleDir = "{{ playbook_dir}}/../artifacts/images"

	}

	if nodes.Flags["registry"] {
		override.ImageRegistriesWithAuth = append(override.ImageRegistriesWithAuth, registryInfo)
	}
	if nodes.Flags["gpu"] {
		override.Gpu.Types = append(override.Gpu.Types, "nvidia")
		override.BuildNameExtra = "-nvidia"
	}

	data, err := yaml.Marshal(&override)
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("overrides/"+name+".yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}
	cmd := exec.Command("kubectl", "create", "secret", "generic", name, "--from-file=overrides.yaml=overrides/overrides/"+name+".yaml")
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}
	cmd = exec.Command("kubectl", "label", "secret", name, "clusterctl.cluster.x-k8s.io/move=")
	output, err = cmd.CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		log.Fatal(err)
	}

}
