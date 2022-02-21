# PKD UP

The PKD UP command is composed of distinct cluster generation phases:

1. Bootstrap Cluster Creation
2. PreprovisonedInventory Object Creation
3. DKP Dry Run Output
4. Cluster Object Parsing and Creation
5. Application of Cluster Objects and Deployment
6. Creation and Pivot of Cluster Controllers 
7. Bootstrap Cluster Destruction
8. Kubeconfig Merging

## Bootstrap Cluster Creation

Internally this runs dkp bootstrap delete and then dkp bootstrap create. This is to ensure that we're always starting with a fresh bootstrap cluster so please be mindful that you will lose any data in your previous bootstrap cluster on pdk up


## PreprovisionedInventory Object Creation

This step creates the PreprovisionedInventory Ojbects for the Control Plane and every set of Node Pools in your cluster.yaml. We also create a secret for your ssh-key at this time. 

## DKP Dry Run Output

This command is essentially what this tool was designed to replace. The dkp create cluster preprovisioned command can deploy an on prem cluster, but it can only support a single node pool, doesn't support KIB overrides and requires a lot of environment variables to be properly set in order for it to run succesfully. 

## Cluster Object Parsing and Creation

We generate the dry run output, configuring the load balancer for the controlplanes and the interface for the control planes, then we write this information to a single file that contains multiple yaml objects. This large yaml file is not very useful as it is hard to manage individual objects, so we split it up into a separate file for every object inside it. Any object that we must edit we discard and instead generate from scratch using the values from cluster.yaml to fill in the values. We also add the Konvoy Image Builder overrides and custom CIDR ranges during this step. 

## Application of Cluster Objects and Deployment 

This step walks through the entire /resources/ directory and applies every yaml object inside that is not a PreprovisionedInventory object as those were previously applied. If you add custom objects to this directory they will be applied at this time. 

## Creation and Pivot of Cluster Controllers

This step waits for every machine to become ready before attempting the pivot operation. You can track the status of the deployment in a separate window via:

```bash 
watch kubectl get machines
```

If you notice that a node is stuck in Pending, take a look at the jobs in the default namespace:

```bash
kubectl get jobs
```

If you see any jobs with a status of Error, delete them so they are restarted. Usually this will resolve your issue and the deployment can continue, but you may also inspect the capp-controller logs. Get it via:

```bash 
kubectl get pods -n cappp-system
```

```bash
[tony@centos cluster-b]$ kubectl get pods -n cappp-system
NAME                                        READY   STATUS    RESTARTS   AGE
cappp-controller-manager-56fcf85446-c2hpc   1/1     Running   0          4h4m
```

Then you can read the logs for more information:

```bash
kubectl logs -n cappp-system cappp-controller-manager-56fcf85446-c2hpc
```

## Bootstrap Cluster Destruction

Once the cluster has successfully pivoted, we delete the bootstrap cluster as it is no longer needed.


## Kubeconfig Merging

This last step fetches the kubeconfig from the newly deployed cluster and merges it with your existing kubconfig stored at ~/.kube/config:

```bash
KUBECONFIG=<cluster-config.conf>:~/.kube/config
```

After they are merged, we write the contents to a file and replace our exisitng kubeconfig with the newly merged one. Then we read in the single context that is stored in the kubeconfig we fetched from our newly deployed cluster and use that to set the default context on the merged kubeconfig which is now located at ~/.kube/config









