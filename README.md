

AIM:
----
To create a singleton active POD and a passive (not receiving/handling any traffic) standby POD which is ready to become active if the active POD goes down.
Due to time critical functionality being executed in the active POD, can't rely on K8s to create a new POD and get the application initialization done within milliseconds. 
Hence, need to keep a standby POD ready beforehand so that the failover happens quite fast.

Also, want to nominate certain containers as critical containers and others as non-critical containers.
If a critical container dies, then the POD needs to be restarted as the initialization of the critical POD is expected to take a lot of time and we better failover
to the standby POD which is ready to become active.

The above functionality should be implemneted as an Internal Controller (ICR) pod and the ICR pod itself should be highly available.
There should be an active and a standby ICR pod similar to the singleton pod described above.


Solution:
---------

The application is running as a singleton POD and is actively receiving traffic via a ClusterIP service.
There is another POD running the same application but working in standby mode. It is not receiving any traffic as it is not a backend pod of the service.

Since labels and annotations apply to the entire POD and not to containers within it, I am using environment variable to mark containers as critical or non-critical.
The env variable "pwcriticality" with a value "critical" determines a critical container.

An Internal Controller (ICR) POD will monitor the POD events happening in the cluster and do the label changes, pod deletion etc as mentioned above.
A standby ICR pod will be created and it will take over the ICR funtionality if the active ICR pod dies.


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


Custom Controller (Internal Controller - ICR):
------------------
The custom controller looks for POD creation and deletion events for PODs having label "app=hapod".
When the replicaset creates the 2 PODs, the custom controller will change the label of one of them to active i.e. "role=active".
Right now, I have selected the one with earliest creation time for becoming active.

Once the role label is changed to active, the Service will select that POD as backend POD and start directing traffic to it.

The other POD will continue to behave as standby and will not be receiving any traffic.

The Internal Controller functionality will run inside a POD, ICR Pod.
This will be created as part of a replicaset with replicase=2.

Failover:
---------

If the active POD dies, the custom controller will get notified of the event and will immediately change the label of the standby POD to active i.e. "role=active".
It will also send a message (used UDP socket for this purpose) to the POD to start behaving as an active POD.

This will make the Service to select this newly active pod as backend POD and start directing traffic to it.

Meanwhile, the replicaset will detect the POD going down and create a new POD to make sure there are 2 replicas always running.
This new POD will have label "role" as standby.

The ICR Pods will behave in a similar manner.
When the 2 ICR Pods are created, the one with a lower aplhabetical Pod name will become active and will execute the ICR functionality.
It will have a label "role" as active.
The other ICR Pod will have label "role" as standby.

When the active ICR Pod dies, the standby ICR pod will receive this notification from the apiServer and will promote itself to become
the active ICR Pod.

Steps of exection:
------------------

We need the following:

- Application POD with a container which listens on a UDP port to receive the "become-active" notification from custom controller.
- The POD contains multiple containers with some of them having environment variable "pwcriticality" set to "critical". 
- ICR binary (as a docker image) will run as a POD inside the cluster.
- Replicaset and Service yaml files with appropriate labels and selectors.


-> The application replicaset and service are defined in the hapod_replicaset.yaml and ha_service.yaml files.

-> The ICR replicaset is defined in icr_replicaset.yaml.
   The icr_sa.yaml, icr_cr.yaml and icr_crb.yaml define the serviceAccount, clusterRole and clusterRoleBinding needed for executing
   the client-go functionality.

-> The udp_server_go.go file contains the application code compiled into the binary "goudps".
   This binary is then used to create a container image "mygoudpsrv:v1" as defined in the file ActiveStandbySingletonPod/udpsrv/Dockerfile.

-> The main.go and podcontroller.go files contain the custom controller logic to manage the labels of the PODs.
   The icr_controller.go file implements the functionality for high availability of the ICR itself.

-> The lifecyclecontainer/ directory contains the executable "lifecycle" generated using the C file lifecyclehook.c.
   This program keeps looking for a file /tmp/killme every 5 seconds and crashes itself when it finds the file.
   This executable is then used to create a container image "lchook:v1" using the Dockerfile file in the same directory.

-> Both the above container images, mygoudpsrv:v1 and lchook:v1 are then used for creating containers in the pod definition given
   in the file replicaset1.yaml file.
   Both the containers have the env variable "pwcriticality" set to "critical" so as to test the scenario where POD should get restarted
   if a critical container dies.



To build application docker images:
-----------------------

cd ActiveStandbySingletonPod/udpsrv

go build // This will build the goudps binary.

cd ActiveStandbySingletonPod/

docker build . -t mygoudpsrv:v1 // This will create the container image and make it available in docker images on local machine.

cd ActiveStandbySingletonPod/lifecyclecontainer/
docker build . -t lchook:v1 // This will create the container image and make it available in docker images on local machine.


To compile the custom controller binary:
----------------------------------------

// The below command will create the binary "icr" in the same directory. Check file go.mod.
cd ActiveStandbySingletonPod/

go build

docker build . -t icr:v1

To run the custom controller as a pod in the cluster:
-----------------------------------------------------

kubectl create -f icr_sa.yaml

kubectl create -f icr_cr.yaml

kubectl create -f icr_crb.yaml

kubectl create -f icr_replicaset.yaml

To run the application pods in the cluster:
-------------------------------------------

kubectl create -f hapod_replicaset.yaml

Kill the critical container in the application pod:
---------------------------------------------------
// After the critical container dies inside the pod, the ICR will kill the pod itself.

kubectl exec -it <active-pod-name> --container critcontainer -- touch /tmp/killme


Kill the active pod:
--------------------

// Use this cmd to find the active pod: kubectl describe pods | grep -e role -e "^Name:"

// Use this cmd to kill either the active application pod or the active icr pod.

kubectl delete pod --force <active-pod-name> 





