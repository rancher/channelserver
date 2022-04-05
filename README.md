# channelserver
This project is a micro-service for exposing multiple release channels for a software project. It is inspired by (and follows the convention of) GitHub’s `/releases/latest` paradigm, which will always redirect to the latest release of a project.

It expands on the `/releases/latest` concept by allowing you to create multiple release channels, each with their own URL that will redirect to the latest release for that channel.

A channel's latest release can be configured by [explicitly pinning it to a version](https://github.com/rancher/channelserver/blob/439813cefa7a0bd048052bcabc7b1c6ad796e97a/channels.yaml#L4) or by [selecting a set of releases via regular expression](https://github.com/rancher/channelserver/blob/439813cefa7a0bd048052bcabc7b1c6ad796e97a/channels.yaml#L11) (in which case the most recently release matching the regex will be resolved as the latest release). For more examples, see the [sample config](https://github.com/rancher/channelserver/blob/master/channels.yaml).

The primary usecase for this project is currently Rancher's [system-upgrade-controller](https://github.com/rancher/system-upgrade-controller), which is used for automating upgrades of k3s and k3os. In the system-upgrade-controller, a user can specify a channel exposed by this sevice as the URL in the plan.spec.channel field.

This service is in proudction for k3s here: https://update.k3s.io/v1-release/channels, which is driven by this config: https://github.com/rancher/k3s/blob/master/channel.yaml. Each channel's `self` link is the URL that will resolve to the latest GitHub release page for that channel.

## Compile and Run 
1. build the binary
```
go build . 
```
2. run the server
```
 ./channelserver --config-key k3s --path-prefix v1-release --channel-server-version v2.3.1
```

3. run the following curl command or open the url in the browser
```
curl 0.0.0.0:8080/v1-release/release 
curl 0.0.0.0:8080/v1-release/channel 
curl 0.0.0.0:8080/v1-release/appdefault
```

## License
Copyright (c) 2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
