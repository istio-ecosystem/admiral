
<p align="center">
  <img src="https://user-images.githubusercontent.com/35096265/65359707-33216900-dbb2-11e9-8622-dc76c3882c02.png" width="500">
</p>


[![CircleCI](https://circleci.com/gh/istio-ecosystem/admiral/tree/master.svg?style=svg)](https://circleci.com/gh/istio-ecosystem/admiral/tree/master) [![codecov](https://codecov.io/gh/istio-ecosystem/admiral/branch/master/graph/badge.svg)](https://codecov.io/gh/istio-ecosystem/admiral)

**Admiral provides automatic configuration and service discovery for multicluster Istio service mesh**

Istio has a very robust set of multi-cluster capabilities.  Managing this configuration across multiple clusters at scale is challenging.  Admiral takes an opinionated view on this configuration and provides automatic provisioning and syncing across clusters.

Admiral defines two custom resources, the [Dependency](./docs/Architecture.md#dependency) and [GlobalTrafficPolicy](./docs/Architecture.md#global-traffic-policy), which are used to configure ServiceEntries, VirtualServices, and DestinationRules on each cluster for each cross-cluster service. This removes the complexity for developers and mesh operators.

![alt text](./docs/diagrams/admiral.svg)

[Admiral Demo](https://www.youtube.com/watch?v=cwQpt1t287c)

## [Docs](./docs/Index.md)

## Who uses Admiral?

Organizations below are **officially** using Admiral. Please send a PR with your organization name if you are using Admiral.

* [Intuit](https://www.intuit.com/)

## Blogs and Presentations

* [Multicluster Istio configuration and service discovery using Admiral](https://istio.io/blog/2020/multi-cluster-mesh-automation/)

* [Stitching a Service Mesh Across Hundreds of Discrete Networks](https://www.youtube.com/watch?v=EWyNbBn1vns)

* [Scaling Service Mesh to an Enterprise Microservices Ecosystem](https://apiworld2019aidevworld2019.sched.com/event/SLIQ/pro-talk-scaling-service-mesh-to-an-enterprise-microservices-ecosystem)

## Collaboration and Communication

[Admiral Slack Channel](https://istio.slack.com/archives/CT3F18T08) - `Note:` This channel is under Istio slack org, please fill out this [form](https://docs.google.com/forms/d/e/1FAIpQLSfdsupDfOWBtNVvVvXED6ULxtR4UIsYGCH_cQcRr0VcG1ZqQQ/viewform) to get access to Istio slack.

## Contributing
Refer to [Contributing doc](./CONTRIBUTING.md)

## Release Cadence

Details can be found [here](./docs/Processes.md)

## Admiral sequence diagram
```mermaid
sequenceDiagram
		autonumber 1
    Service/VirtualService Handler->>+Rollout/Deployment Handler: Add/Update/Delete events
    loop
			autonumber 1
			GTP Handler->>GTP Handler: Add/Update/Delete events
    end
		autonumber 1
    GTP Handler ->> ServiceEntry Handler: Add/Update
		loop
				autonumber 1
        Rollout/Deployment Handler->>Rollout/Deployment Handler: Add/Delete events of rollout/deployment
    end
		autonumber 1
		Rollout/Deployment Handler->>ServiceEntry Handler: Add/Update
    autonumber 2
    ServiceEntry Handler->>RemoteControllers: Fetch All Cluster Controllers
		rect rgb(255, 255, 220)
	    loop
		    ServiceEntry Handler->>K8sAPI 1..N: For each cluster, get corresponding service object
				K8sAPI 1..N-->>ServiceEntry Handler: Continue if service does not exist for deployment/rollout
				K8sAPI 1..N-->>ServiceEntry Handler: Build list of source services
	    end
		end
	  rect rgb(255, 255, 220)
	    loop
				ServiceEntry Handler->>K8sAPI 1..N: Derive SE from each service in the list
				ServiceEntry Handler->>GTP Cache: Derive DR from GTP
			  rect rgb(204, 255, 204)
			    loop
				    ServiceEntry Handler->>K8sAPI 1..N: Add/Update SE/DR in source clusters
					end
		    end
	    end
		end

    ServiceEntry Handler->>DependencyCache: Fetch dependent clusters
		rect rgb(204, 255, 204)
	    loop
				ServiceEntry Handler->>K8sAPI 1..N: Add/Update SE/DR in dependent clusters
			end
		end
```
