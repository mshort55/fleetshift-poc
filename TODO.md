# TODO

## Design

- [ ] Incorporate "root" service account (locked up keys, rarely used) best practice? ([FCN-187](https://redhat.atlassian.net/browse/FCN-187))
- [ ] Look at workload identity federation kinds of techniques for authenticating the platform or authenticating agents ([FCN-140](https://redhat.atlassian.net/browse/FCN-140), [FCN-180](https://redhat.atlassian.net/browse/FCN-180), [FCN-200](https://redhat.atlassian.net/browse/FCN-200))
- [ ] Should RBAC "push" (bootstrapping) be done by deployments in which the pool is everything in that workspace? And then new clusters are new targets which trigger the pipeline again? This is pretty elegant and follows existing authorization model. ([FCN-181](https://redhat.atlassian.net/browse/FCN-181))
- [ ] I think an explicit tenant notion is needed in the managed cluster case (can possibly be sharded but instances are still managed by provider) ([FCN-182](https://redhat.atlassian.net/browse/FCN-182))
- [ ] What happens if agent is offline during delivery? Does it stall the whole stage? ([FCN-192](https://redhat.atlassian.net/browse/FCN-192))
- [ ] How do we securely store tokens used for deployment while being able to place to new targets if placement is updated? I wanted to encrypt per target but that would require reauth on new placement (maybe that is one option). Do we require refresh tokens for this, or store the initial JWT encrypted with a platform key? ([FCN-201](https://redhat.atlassian.net/browse/FCN-201))
- [ ] How should IdP resolution work for new clusters? In a single tenant situation with a single IdP this is easy. But what about when this is a multitenant provider? ([FCN-202](https://redhat.atlassian.net/browse/FCN-202))
- [ ] Look at upgrades (rolling out updates to deployments themselves) (see related agent conversation, "Controlled fleet-wide upgrades with FleetShift")

## Functionality

- [ ] Fleetlet installation into kind cluster ([FCN-192](https://redhat.atlassian.net/browse/FCN-192))
- [ ] Pagination / filter / etc
- [ ] Revisit failed deployment when no targets – what if it is invalidated? (manifest update) Do we have/need a signal for when new targets might match because they've been newly registered?
- [ ] What if multiple targets match manifest types? Initial target pool needs to be a bit more specific and therefore flexible. Maybe we want an "InitialPool" kind of abstraction in addition to the placement strategy. ([FCN-193](https://redhat.atlassian.net/browse/FCN-193))
    - f(deployment) -> targets (initial)
    - f(targets) -> targets (strategy)

## Implementation detail (nontrivial)

- [ ] Consider the durable workflow invalidation path – the current "always running" model doesn't work well with DBOS. Do we care? See transcript "Workflow engine behavior during deployment events"
- [ ] Not sure about the credential design (should we assume one raw token? what about other token types?)

## Code / Trivial

- [ ] Rethink where generated proto types go
- [ ] Revisit workflow contract tests because I think these are just testing the same workflow logic tested elsewhere (so maybe just combine & use to test workflow implementations & workflow logic)
- [ ] In process delivery agents need to also use durable workflows (in their own addon package)
- [ ] The feetctl token output should probably be treated more like standard output rather than custom
