# Steps to release admiral

## Prerequisites

* You are an admin on the admiral github project (to cut the release)
* You have access to admiral slack channel and AdmiralProj twitter account (to make release announcements)
* [Validate that the master branch build is passing](https://circleci.com/gh/istio-ecosystem/admiral/tree/master)

## Draft the release notes

The release notes is maintained [here](ReleaseNotes.md). Open a PR to the release notes ahead of time and get it reviewed.

### What to cover
* Any major features and examples on how to use them
* Bug fixes
* Any other callouts, like support for newer versions of Istio/Cloud Vendors

### How to find the issues/commits
* You can find the [issues associated with a milestone](https://github.com/istio-ecosystem/admiral/issues?q=is%3Aissue+milestone%3Av1.0+) to get started.
* [Commits](https://github.com/istio-ecosystem/admiral/compare/v1.1...master) between last release (the link assumes `v1.0`) and `master` branch


## Draft a new release on github.com 

* Go to [new release page](https://github.com/istio-ecosystem/admiral/releases/new)
* Create a release with the following:
 Tag version = tag that matches a milestone being released (Ex: v1.1).
 Release title = same as Tag version
 Description = <link to the relese notes> Ex: https://github.com/istio-ecosystem/admiral/blob/master/docs/ReleaseNotes.md#v11 (note that v11 comes from tag v1.1)

## Final checklist

* Make sure the admiral release tag is published [here](https://hub.docker.com/r/admiralproj/admiral/tags)
* Merge the release notes PR
* Make sure the links in the release notes work
* Make announcements
    - On istio [Admiral slack channel](https://istio.slack.com/archives/CT3F18T08)
    - Tweet about it [Admiral Twitter](https://twitter.com/AdmiralProj)
