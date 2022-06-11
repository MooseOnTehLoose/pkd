# Quick start

You should ensure you have the DKP and Kommander CLI present in your working directory before attempting to use PKD. You will also need the ssh-key used to connect with the hosts you plan on provisioning. 

## Initialize

Grab the latest build of PKD here:

https://github.com/MooseOnTehLoose/pkd/releases

Move it to your working directory and make it executable:

```bash
chmod a+x pkd
```

Generate a new cluster.yaml 

```bash
pkd init
```

## Editing Cluster.yaml

cluster.yaml has a few key sections that are mandatory in order for your cluster to spin up properly. Lets take a look at a default cluster yaml file:

```yaml
metadata:
    name: pkd-default-cluster
    sshuser: user
    sshprivatekey: id_rsa
    interfacename: ens192
    loadbalancer: 10.0.0.10
registry:
    host: registry-1.docker.io
    username: "password"
    password: "user"
controlplane:
    hosts:
        controlplane1: 10.0.0.11
        controlplane2: 10.0.0.12
        controlplane3: 10.0.0.13
    flags:
        registry: true
nodepools:
    md-0:
        hosts:
            worker1: 10.0.0.14
            worker2: 10.0.0.15
            worker3: 10.0.0.16
            worker4: 10.0.0.17
        flags:
            registry: true

```

### Metadata stores information specific to this cluster shared by all nodes:
- name: The name of the cluster
- sshuser: The user associated with the ssh key required for deployment
- sshprivatekey: The ssh-key used to connect to your hosts
- interfacename: This is used by the Control Plane Loadbalancer, it should be the value of the interface on your control planes you will use
- loadbalancer: This should be an unused IP address in the same subnet as your Control Plane nodes

### Registry stores information abouut the Docker Image Registry that you will use to pull images.
- host: The address of the registry. Docker Hub by default
- username: User for the Registry
- password: Can be a password or token used to authenticate to your registry

### ControlPlane stores information about your Control Plane hosts
- hosts: This is a list of your control plane hosts. Each control plane must have a unique name
- flags: This is a list of flags that all have a value of true or false. They default to false if not specified. 

### NodePools is a list of NodePools that each have their own hosts and flags. 
You can name your nodepools whatever you want, although DKP cli defaults to the naming convention md-<X>. 
It may be helpful to give your GPU enabled nodes a node-pool name such as gpu-md-1
You can have any number of nodepools to separate your workers into deployment groups but every worker must have a unique Name and IP across all nodepools!
    
## Deploying a DKP 2 Cluster
Once you have customised your cluster yaml, its time to generate all resources required and then apply them to the bootstrap cluster. PKD will take care of all of this for you:
    
```bash 
pkd up
```
When pkd up is finished, you should be left with a DKP 2.X cluster ready to use, kubectl preconfigured. You can now go on to deploying Kommander and Kaptain!

For a detailed view of whats going on under the hood, see: [PDK UP](pkdUP.md#).

New in v0.1.0-beta.2 is a cowboy mode for anybody who wants to manually edit the objects under /resources before they are applied. You can manually customize PKD for any feature it doesn't yet support automating. 
    
```bash
pkd up yee-haw
```
   
Good Luck Cowboy!
