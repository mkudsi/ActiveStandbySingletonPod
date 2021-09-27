

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
