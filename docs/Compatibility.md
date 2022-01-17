# Compatibility

## Version compatibility by Istio & K8s
The below information is based on the testing done, please submit a PR if you have it working for versions outside the ones listed below.

| Admiral Version   | Min. Istio Version    | Max. Istio Version    | Min. K8s Version  |  Max. K8s Version
|:-----------------:|:---------------------:|:---------------------:|:-----------------:|:-----------------:
v0.1-beta           | 1.2.3                 | 1.4.6                 | 1.13              | 1.14.2
v0.9                | 1.2.3                 | 1.5.1                 | 1.13              | 1.18.0
v1.0                | 1.2.3                 | 1.5.6                 | 1.13              | 1.18.0
v1.1                | 1.5.7                 | 1.7.4                 | 1.13              | 1.18.0
v1.2                | 1.8.6                 | 1.12.1                | 1.18              | 1.22

## Admiral feature support by Istio Version

| Admiral Version   | Syncing   | Dependency    | Global Traffic Policy
|:-----------------:|:---------:|:-------------:|:--------------------:
v0.1-beta           | Yes       | Yes           | No
v0.9                | Yes       | Yes           | Yes (requires Istio 1.5.1 or higher)
v1.0                | Yes       | Yes           | Yes (requires Istio 1.5.1 or higher)
v1.1                | Yes       | Yes           | Yes (requires Istio 1.5.1 or higher)
v1.2                | Yes       | Yes           | Yes (requires Istio 1.8.6 or higher)

## Tested cloud vendors

| Admiral Version   | Cloud vendor
|:-----------------:|:---------:
v0.1-beta           | AWS       
v0.9                | AWS
v1.0                | AWS, GCP, Azure
v1.1                | AWS, GCP, Azure
v1.2                | AWS, GCP, Azure

`Note`: Please submit a PR if admiral was tested on other cloud vendors       