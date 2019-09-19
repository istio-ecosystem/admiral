# Contributing to Admiral

## Pre-requisite
1.  We'll need at least 2 kubernetes clusters running istio to see the power of Admiral :).
2.  The ingressgateway loadbalancer is accessible from each cluster
3.  The admiral repository is cloned: ```git clone https://github.com/istio-ecosystem/admiral.git```
4.  GoLand has been downloaded ([download it here](https://www.jetbrains.com/go/download/download-thanks.html?platform=mac))
## Setting up Admiral for development
1. Decide the cluster admiral will run on. This is will be referred to as the `local cluster`. The `local cluster` is where we will deploy the client.
   Set the env variable `LOCAL_CLUSTER` to point to the local cluster's KUBECONFIG.  

    ```export LOCAL_CLUSTER=<pwd>/admins@your-local.cluster.k8s.local```
2. Our other cluster will be referred to as the`remote cluster`. The `remote cluster` is where we will deploy a remote service that gets accessed by the client running in the `local cluster`, using an admiral generated service name.
   Set the env variable `REMOTE_CLUSTER` to point to the remote cluster KUBECONFIG.
    
    ```export REMOTE_CLUSTER=<pwd>/admins@you-remote.cluster.k8s.local```

3. Set up namespaces needed for the `remote cluster` and `local cluster`:
    ```
    export KUBECONFIG=$REMOTE_CLUSTER
    kubectl apply -f install/admiral/admiral-sync-ns.yaml
    
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f install/admiral/admiral-sync-ns.yaml
    ```
    `Note:` You can also override these namespaces when you start admiral
4. Provision a service account on `local cluster` and `remote cluster`. Admiral will use it to talk to the k8s api server between clusters.
    ```
    export KUBECONFIG=$REMOTE_CLUSTER
    kubectl apply -f install/admiral/remote-sa.yaml
    
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f install/admiral/remote-sa.yaml
    ```
5. Run the following shell scripts (this fetches service account's secret from a cluster and drops it into `admiral-secret-ns` namespace in local cluster):
    ```
    sh test/scripts/cluster-secret.sh $LOCAL_CLUSTER $REMOTE_CLUSTER
    sh test/scripts/cluster-secret.sh $LOCAL_CLUSTER $LOCAL_CLUSTER
    ```
6. Create dependencies and global traffic policies CRDs in local cluster and create global traffic policy in remote cluser
    ```
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f admiral/crd/dependency.yaml
    kubectl apply -f admiral/crd/globalTraffic.yaml   
    
    export KUBECONFIG=$REMOTE_CLUSTER
    kubectl apply -f admiral/crd/globalTraffic.yaml 
    ```
7.  Open the admiral project in GoLand
    * navigate to the `Run` tab and click `Edit Configurations`
    * in the field `Program arguments` add the following parameters:
  
    ``` 
    --kube_config <path_to_kube_config_of_local_cluster>
    ```
    * check run after build
    * specify `-i` in go `Go tool arguments`
    
8. Run admiral/admiral/cmd/admiral/cmd/main.go  to make sure admiral started successfully
9. Create namespace in both clusters
    ```
    export KUBECONFIG=$REMOTE_CLUSTER
    kubectl apply -f examples/mc-namespace.yaml
    
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f examples/mc-namespace.yaml
    ```
10. Update examples/serverv1.0.yaml with the correct role, policy id, and identity. Deploy serverv1 on remote cluster
    ```
    export KUBECONFIG=$REMOTE_CLUSTER
    kubectl apply -f examples/serverv1.0.yaml -n mc
    ```
11. Update examples/client.yaml with the correct role, policy id, and assetId. Deploy client on local cluster
    ```
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f examples/client.yaml -n mc 
    ```
12. Create dependency record in the `local cluster`
    ```
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl apply -f examples/dependency_record.yaml  
    ```

13. At this point, the client app can discover names for server by filtering Istio Service Entries with identity label.
    Shell into client to verify call to server is successful
    ```
    export KUBECONFIG=$LOCAL_CLUSTER
    kubectl exec -ti <client_pod_name> -c client sh -n mc
    
    curl -v stage.server.global/hello
    ```
