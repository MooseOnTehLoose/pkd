# Quick start

You should ensure you have the DKP and Kommander CLI present in your working directory before attempting to use PKD. You will also need the ssh-key used to connect with the hosts you plan on provisioning. 

## Initialize

Grab the latest build of PKD here:

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
    auth: ""
    identityToken: ""
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
