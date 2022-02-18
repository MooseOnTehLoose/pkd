# 3 Control Plane Nodes, 2 Worker Node Pools, 1 with GPUs

```yaml
metadata:
    name: data-science-cluster
    sshuser: jbond
    sshprivatekey: id_rsa
    interfacename: ens192
    loadbalancer: 10.4.6.40
registry:
    host: registry-1.docker.io
    username: "charles"
    password: "banana"
controlplane:
    hosts:
        controlplane1: 10.4.6.41
        controlplane2: 10.4.6.42
        controlplane3: 10.4.6.43
    flags:
        registry: true
nodepools:
    md-0:
        hosts:
            worker1: 10.4.6.44
            worker2: 10.4.6.45
            worker3: 10.4.6.46
            worker4: 10.4.6.47
            worker5: 10.4.6.48
            worker6: 10.4.6.49
            worker7: 10.4.6.50
            worker8: 10.4.6.51
        flags:
            registry: true
    md-1:
        hosts:
            worker1: 10.4.6.52
            worker2: 10.4.6.53
        flags:
            gpu: true
            registry: true


```

# 3 Control Plane Nodes, 1 Worker Node Pools with Docker Hub Credentials and custom Pod and Service Subnets

```yaml
metadata:
    name: home-cluster
    sshuser: carl
    sshprivatekey: id_rsa
    interfacename: ens192
    loadbalancer: 10.4.8.40
    podsubnet: 192.168.252.0/24
    servicesubnet:  192.168.253.0/24
registry:
    host: registry-1.docker.io
    username: "carl"
    password: "banana"
    auth: ""
    identityToken: ""
controlplane:
    hosts:
        controlplane1: 10.4.8.41
        controlplane2: 10.4.8.42
        controlplane3: 10.4.8.43
    flags:
        registry: true
nodepools:
    md-0:
        hosts:
            worker1: 10.4.8.44
            worker2: 10.4.8.45
            worker3: 10.4.8.46
            worker4: 10.4.8.47
        flags:
            registry: true
```


