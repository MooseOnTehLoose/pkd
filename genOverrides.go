package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

func genOverride2_6_0(name string, nodes NodePool, registryInfo Registry, airgap bool) {

	override := kibOverride{}

	// Full Path with http(s)://
	registryURL := registryInfo.Host

	//Strip Protocol or we get an error, do this more elegantly later on
	registryADDR := strings.ReplaceAll(registryURL, "https://", "")
	registryADDR = strings.ReplaceAll(registryADDR, "http://", "")

	//Example: https://harbor-registry.com/registry
	//If no port specified, insert /v2/ after last period and before first /
	//
	//			https://harbor-registry.com/v2/registry
	//
	//If port specifed, do nothing
	//
	// Special Case: Docker Creds for docker.io
	//
	// registry-1.docker.io
	//
	//

	//if this address doesn't a port
	if !strings.Contains(registryADDR, ":") {
		//don't insert a /v2/ for docker's official registry
		if !(registryADDR == "registry-1.docker.io") {
			// https://harbor-registry.
			periodIndex := strings.LastIndex(registryURL, ".")

			// https://harbor-registry.
			firstHalf := registryURL[:periodIndex]
			// com/registry
			secondHalf := registryURL[periodIndex:]

			secondHalf = strings.Replace(secondHalf, "/", "/v2/", 1)

			registryURL = firstHalf + secondHalf

			//for the auth section we need to remove the sub path

			registryADDR = registryADDR[:strings.LastIndex(registryADDR, "/")]
		}
	}

	// If this is an Air Gap Registry override
	if airgap && nodes.Flags["registry"] {

		override.DefaultImageRegistryMirrors.DockerIo = registryURL
		override.DefaultImageRegistryMirrors.Wildcard = registryURL
		override.ImageRegistriesWithAuth = append(override.ImageRegistriesWithAuth,
			struct {
				Host          string "yaml:\"host,omitempty\""
				Username      string "yaml:\"username,omitempty\""
				Password      string "yaml:\"password,omitempty\""
				Auth          string "yaml:\"auth,omitempty\""
				IdentityToken string "yaml:\"identityToken,omitempty\""
			}{
				Host:          registryADDR,
				Username:      registryInfo.Username,
				Password:      registryInfo.Password,
				Auth:          "",
				IdentityToken: "",
			})
	}

	// If this is a regular Registry Override
	if !airgap && nodes.Flags["registry"] {

		override.ImageRegistriesWithAuth = append(override.ImageRegistriesWithAuth,
			struct {
				Host          string "yaml:\"host,omitempty\""
				Username      string "yaml:\"username,omitempty\""
				Password      string "yaml:\"password,omitempty\""
				Auth          string "yaml:\"auth,omitempty\""
				IdentityToken string "yaml:\"identityToken,omitempty\""
			}{
				Host:          registryADDR,
				Username:      registryInfo.Username,
				Password:      registryInfo.Password,
				Auth:          "",
				IdentityToken: "",
			})
	}

	// GPUs are not supported in Air Gap in 2.2.0
	if nodes.Flags["gpu"] && !airgap {
		override.Gpu.Types = append(override.Gpu.Types, "nvidia")
		override.BuildNameExtra = "-nvidia"
	}

	data, err := yaml.Marshal(&override)
	if err != nil {
		log.Fatal(err)
	}
	err = os.WriteFile("overrides/"+name+".yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}
	cmd := exec.Command("kubectl", "create", "secret", "generic", name, "--from-file=overrides.yaml=overrides/"+name+".yaml")
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
