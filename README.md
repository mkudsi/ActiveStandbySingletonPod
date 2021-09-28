

AIM:
----
To create a singleton active POD which has a passive (not receiving/handling any traffic) standby POD which is ready to become active once the active POD goes down.
Due to time critical functionality being executed in the active POD, can't rely on K8s to create a new POD and get the application initialization done within milliseconds. 
Hence, need to keep a standby POD ready beforehand so that the failover happens quite fast.

Also, want to nominate certain containers as critical containers and others as non-critical containers.
If a critical container dies, then the POD needs to be restarted as the initialization of the critical POD is expected to take a lot of time and we better failover
to the standby POD which is ready to become active.



Solution:
---------

The application is running as a singleton POD and is actively receiving traffic via a ClusterIP service.
There is another POD running the same application but working in standby mode. It is not receiving any traffic as it is not a backend pod of the service.

Since labels and annotations apply to the entire POD and not to containers within it, I am using environment variable to mark containers as critical or non-critical.
The env variable "pwcriticality" with a value "critical" determines a critical container.


Replicaset:
-----------
To achieve this, a replicaset with image of the application and replicas=2 is defined.
The replicaset will ensure that there are always 2 instances of the application POD always up and running.
The replicaset will add 2 labels to the PODs:

app=hapod
role=standby

So by default, both PODs will have role as standby.

Service:
--------
A ClusterIP service is created which has the following selector criteria:
"app=hapod AND role=active".

Since both the PODs created by the replicaset have app=hapod, but role=standby, initially, the service will not find any matching pods.


Custom Controller:
------------------
The custom controller looks for POD creation and deletion events for PODs having label "app=hapod".
When the replicaset creates the 2 PODs, the custom controller will change the label of one of them to active i.e. "role=active".
Right now, I have selected the one with earliest creation time for becoming active.

Once the role label is changed to active, the Service will select that POD as backend POD and start directing traffic to it.

The other POD will continue to behave as standby and will not be receiving any traffic.


Failover:
---------

If the active POD dies, the custom controller will get notified of the event and will immediately change the label of the standby POD to active i.e. "role=active".
It will also send a message (used UDP socket for this purpose) to the POD to start behaving as an active POD.

This will make the Service to select this newly active pod as backend POD and start directing traffic to it.

Meanwhile, the replicaset will detect the POD going down and create a new POD to make sure there are 2 replicas always running.
This new POD will have label role as standby.


Steps of exection:
------------------

We need following:

- Application POD with a container which listens on a UDP port to receive the "become-active" notification from custom controller.
- The POD contains multiple containers with some of them having environment variable "pwcriticality" set to "critical". 
- Custom controller binary (or docker image) which will run on the host (or as a POD inside the cluster).
- Replicaset and Service yaml files with appropriate labels and selectors.


-> The replicaset and service are defined in the replicaset1.yaml and service1.yaml files.

-> The udp_server_go.go file contains the application code compiled into the binary "goudps".
   This binary is then used to create a container image "mygoudpsrv:v1" as defined in the file ActiveStandbySingletonPod/Dockerfile.

-> The main.go and podcontroller.go files contain the custom controller logic to manage the labels of the PODs.

-> The lifecyclecontainer/ directory contains the executable "lifecycle" generated using the C file lifecyclehook.c.
   This program keeps looking for a file /tmp/killme every 5 seconds and crashes itself when it finds the file.
   This executable is then used to create a container image "lchook:v1" using the Dockerfile file in the same directory.

-> Both the above container images, mygoudpsrv:v1 and lchook:v1 are then used for creating containers in the pod definition given
   in the file replicaset1.yaml file.
   Both the containers have the env variable "pwcriticality" set to "critical" so as to test the scenario where POD should get restarted
   if a critical container dies.



To build docker images:
-----------------------

cd ActiveStandbySingletonPod/udpsrv
go build // This will build the goudps binary.

cd ActiveStandbySingletonPod/
docker build . -t mygoudpsrv:v1 // This will create the container image and make it available on local machine.

cd ActiveStandbySingletonPod/lifecyclecontainer/
docker build . -t lchook:v1


To compile the custom controller binary:
----------------------------------------

// The below command will create the binary "ccpod" in the same directory. Check file go.mod.

cd ActiveStandbySingletonPod/

go build

// We can then run the ./ccpod binary on the host machine itself.


To run the custom controller as a pod in the cluster:
-----------------------------------------------------


// If required, we can also run the custom controller as a pod in the cluster by making a container image out of it.
// In that case, we need to define clusterrole and clusterrolebinding in the "default" namespace for the "default" service account.
// e.g.


kubectl create serviceaccount -n default ccpod-sa --dry-run=client -oyaml > ccpod_sa.yaml

kubectl create clusterrole ccpod-cr --resource=service,pod --verb=list,watch,create,get,update --dry-run=client -oyaml > ccpod_cr.yaml

kubectl create clusterrolebinding ccpod-crb --clusterrole ccpod-cr --serviceaccount default:ccpod-sa --dry-run=client -oyaml > ccpod_crb.yaml


kubectl create -f ccpod_sa.yaml

kubectl create -f ccpod_cr.yaml

kubectl create -f ccpod_crb.yaml

Now, in the container spec in the pod/deployment/replicaset yaml file, specify the service account name:
  
spec:

  containers:

  - image: ubuntu
  
  serviceAccountName: ccpod-sa <<<<<<<<<<<<<<<<<<<



