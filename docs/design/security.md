# Security model

Principles:

- Minimize built-in trust of platform components. Some trust is unavoidable (bootstrapping, token vending), but it should follow an auditable least-privilege model. No god-mode service accounts or keys.
- End to end user identity everywhere – auditable, no confused deputy

## Target credential model

The delivery target plugin gets a say in what credential presentation it should get. So if its a k8s agent, it needs the user's ID token. etc.

How does this work for the service account delegation discussed later? The delivery agent in this case knows to call the token request. Its input is only identity information about the user.

Other reasonable contracts could be "give me the access token" or "I need an access token for X" (so we try to token exchange if we can, for example).

For APIs that leverage federation, the delivery agent handles:

- AWS: Ask for ID token or SAML assertion. AssumeRoleWith*Identity -> sigv4
- GCP: Ask for ID token, token exchange -> GCP token
- etc...

As a fall back these agents could get vault credentials for a service account perhaps, but we want to work off the end user.

## Doable

- We can definitely use the ID token end to end, assuming a common IdP trust and reused client IDs across clusters and the platform. This works for synchronous / short run operations, limited by token lifespan.
- We can definitely query inventory and do platform-side operations securely
- We can sync RBAC on the platform side to kubernetes, assuming we use the user's identity to establish RBAC in the managed cluster. This requires we bootstrap new clusters with the right privileges.
- We can run deployments for as long as we have a token. We can pause deployments waiting for reapproval.

## Things you could do if you can customize the target (e.g. kube distro)

- Validate access tokens instead of ID tokens
- Take into account access token scope (or beyond, resource identifiers, etc)

## Challenges

- Git ops – GitOps has a platform level indirection: the git repo is the authority, and the platform applies from there. Some tools may support tenant-specific service accounts or impersonation.
- Audience scoping – if we want to scope tokens to particular clusters, we need separate audiences for those. More IdP configuration to do. Hard to make dynamic. Token Exchange (RFC 8693) can address this: exchange a platform-audience token for a target-audience token at the IdP. The IdP controls policy (which exchanges are allowed, for which audiences). This avoids per-cluster client IDs but requires IdP support (Keycloak, Dex have it; Auth0/Okta partial).
- Reconciliation – this is similar to the gitops challenge.
- Permission tracking – when a delegation service account's RBAC should track the creating user's permissions over time.

## Bootstrapping targets

When targets (e.g. clusters) are bootstrapped we may necessarily have elevated privileges at that point for that target (e.g. a kubeconfig or a privileged user). Under that identity we assume we can bootstrap other configs like RBAC syncing. These could perhaps be their own deployments or part of the cluster deployment itself.

For the delegation SA model, bootstrapping also provisions the platform's own identity in the cluster. Its service account may get tight impersonation permissions (to impersonate delegate SAs). This is the one piece of unavoidable platform trust, but it's scoped and auditable.

## Long running rollouts (e.g. approval gates) / deployment invalidation

Deployments are long-lived intents in a sense. The rollout may take arbitrarily long. An invalidation signals can come arbitrarily late. This retriggers the deployment.

We can confidently detect these signals but require re-approval to continue. That's maybe not so bad actually but does mean lots of approval baby sitting. What you want is essentially some durable tightly scoped approval that tracks the user's own permissions over time.

So how to store durable tightly scoped approval?

### Token passthrough (synchronous baseline)

The simplest model: the user's bearer token is passed through to the target. Full end-to-end user identity. Works while the token lives. Not sure if we can avoid storing this or if we can use workflow affinity to try and just use a token in memory.

When the token expires mid-rollout, or on workflow replay, the deployment transitions to PausedAuth and waits for an authorized user to resume it with a fresh token. Any authorized user can resume – this is approval-gate semantics for free.

PausedAuth is the universal fallback for all credential models: whenever credentials are insufficient, the deployment pauses rather than failing.

### Service accounts specifically for delegation

When something is long running, the user creates a service account dedicated to run on their behalf, with a scoped subset of their permissions.

The provisioning flow is synchronous (while the user is present):

1. User creates a deployment targeting cluster X
2. The platform, using the user's own token, creates a ServiceAccount + Role + RoleBinding in the target cluster
3. K8s prevents privilege escalation: the RBAC API rejects RoleBinding creation if the user doesn't hold the permissions being bound. The user can only delegate authority they actually have.
4. User's token is discarded after provisioning. Never stored.

The platform then impersonates the service account using its service account identity. This is a small improvement over TokenRequest:

- Impersonation is auditable; token request looks indistinguishable from any other actor with the service account
- There is no additional token that can be used for anything else; that needs to expire, etc.

Ideally: 

- Something expires these over time
- When the user's permissions restricts to less than their shadow service accounts, it automatically restricts the permissions of those service accounts

You could also "just" create specific service accounts to run workloads that you wanted long-running, with strict permissions. If they ever tried to escape that, the deployment pauses for approval.

Trade-offs:

- The target sees the service account identity, not the user. User identity is in the platform's audit log, correlatable via SA naming/annotations but not cryptographically bound.
- Permission drift: if the creating user loses access, the SA retains its grants until explicitly reconciled. We may be able to eagerly cascade permission changes done by the platform to SAs associated with the user.
- K8s-specific pattern. Other targets need equivalents (IAM AssumeRole for AWS, Managed Identity for Azure, etc).

### Refresh tokens

These are credentials and tough to store. This is an alternative path to delegation SAs, not a complement. It preserves end-to-end user identity at the target (the refreshed token IS the user's token), but requires advanced IdP features.

Ideally you'd:

- Sender constrain them (DPoP, RFC 9449). This makes the platform privileged but only its protected private key. Leaked credentials are not a problem. Sender constrained refresh tokens have some support. It would require the backend to be a confidential client and not the frontend. That can complicate CLI integration. Maybe you only approve these long lived flows through the browser, though. It's a few-time operation.
- Scope them. This can be hard because it requires more IdP configuration e.g. client per cluster which could be awful without automation. And automating that is itself difficult to set up (dynamic client registration / aud configuration). Plus you'd want token exchange of some kind or the original aud needs to include every cluster.

Refresh tokens shine when: (a) the IdP supports sender constraints and flexible token exchange (rare in practice), and (b) the targets work well with proper OAuth (access tokens, transaction tokens). For K8s with OIDC auth, delegation SAs are simpler and avoid the stored-secret problem entirely.

### Constrained impersonation

This is conceptually similar to the above, but means the platform directly impersonates the user. The fundamental problem: K8s impersonation lets the impersonator assert group membership, and K8s has no way to verify those assertions. Even with constrained impersonation (limiting which users can be impersonated via `resourceNames`), the impersonator can claim arbitrary groups for that user. If the platform can impersonate group "admins", it can put any user in that group regardless of their actual membership. These are unverifiable claims about a user.

With token passthrough, the IdP is the authority on claims – groups are in the token, cryptographically signed by the IdP. With impersonation, the platform is the authority. This is a fundamentally weaker trust model for any environment where group-based authorization matters.

## IdP orchestration

In various scenarios, we could benefit from specific IdP configuration:

- Per cluster client IDs (audiences)
- Permission-level scoping (assuming you have an authorizer which takes this into account)
- If an IdP can handle the refresh token route... setup for that
- Token exchange (RFC 8693) for audience swapping without per-cluster client IDs
- CAEP/Shared Signals Framework (SSF) for real-time session revocation and permission change events

## Git ops models

### Long lived authority

This assumes we can store something per user like a scoped refresh token. There are many challenges along this path but technically securable with a sufficiently advanced IdP and configuration.

1. Signed commit establishes authn for a change
2. User authorizes server to run changes on their behalf w/ scoped token with particular session limits
3. Applying change runs with user's own identity & applies with their own token

This could have a few models:
- apply runs under an authorized user for the deployment, but the user's identity is used to authorized a change to the deployment
- apply runs under the authorized user of the change, regardless of who originally created the deployment
- apply runs under an authorized user for the deployment, and whether or not the user can edit it is up to git repo <- this is broken

If a change in git is not authorized, what's the feedback loop for that? how do we get back in sync?

One thing that could help matters is if there was a CI check that ran authorization through on the platform level – this could probably catch a lot.

The bigger challenge is securely storing longer lived credentials. See "Refresh tokens" above.

### Signed intent

A more promising model: something cluster-side that operates on "signed intents."

1. A manifest in git is accompanied by a signature and a revision (ideally w/ provenance via hash)
2. The server is only authorized to put "intent" resources into a cluster
3. A controller or admission validates the signature, extracts the original user, authorizes against that, and unwraps the manifest and applies that if 
succesful.

Signing uses keyless signing (Fulcio/cosign model): the user proves OIDC identity to a CA, gets a short-lived certificate binding their identity to an ephemeral signing key. The signature is verifiable long after the token expires via the certificate chain and transparency log.

The architecture separates concerns cleanly:

- The platform has write RBAC for signed intents only (it makes API calls to the cluster)
- The validating webhook gates every write: is this manifest signed by an authorized user?
- The platform can't apply unsigned manifests, can't forge signatures
- The webhook has no write access – it only validates

The platform's write authority is contingent on user signatures. A compromised platform (short of cluster-admin level compromise that could disable the webhook) can't apply anything unauthorized.

A _webhook_ is still privileged (it can block any admission), but its privilege is narrow: verification-only, no writes, auditable, `failClosed`. A controller would be able to write anything. Signature verification built into the API server would be ideal. Signature through validation can be problematic due to mutating web hooks.

#### Signed intent beyond GitOps

Could the deployment itself be the "durable tightly scoped approval" via signing? Two models:

**Eager signing**: generate all manifests upfront, user signs the rendered output, deliver signed artifacts. No provenance chain needed – the signed artifact IS the applied artifact. Clean. But every invalidation requires re-generation, re-review, and re-signing. The user must be present for every invalidation, which is operationally equivalent to PausedAuth. The benefit over PausedAuth is the trust model: cryptographic proof of intent at the target, not just "the platform had a valid token."

**Lazy signing**: user signs the deployment spec, platform generates manifests just-in-time. Invalidation can proceed without the user. But now the platform is in the rendering trust chain – the target must trust that the platform faithfully translated the signed spec into these specific manifests. This requires a provenance chain (spec signature + rendering attestation) and reintroduces platform trust for correctness, though not for identity.

Eager signing is the simpler and more honest model but converges to PausedAuth UX for invalidation. Lazy signing avoids the UX problem but reintroduces trust. Neither is strictly better than delegation SAs for the invalidation case.

Signed intent is most compelling for GitOps (manifests are already in git, already reviewed, signing is natural) and as a trust-model upgrade for environments where cryptographic proof of user intent matters. For interactive long-running deployments, delegation SAs + PausedAuth is the pragmatic choice.

#### Open questions

- SubjectAccessReview in the webhook needs the user's groups. Fulcio certificates typically carry `sub` and `iss`, not groups. The webhook may need to query the IdP for group membership or use a synced group mapping.
- Signed intent is viable for K8s (admission webhooks are a natural fit). For other targets, it's a lot to ask – probably K8s-specific.

## Practical architecture summary

For K8s targets, the layered model:

| Scenario | Mechanism | User identity at target | User presence needed |
|----------|-----------|------------------------|---------------------|
| Synchronous / short-lived ops | Token passthrough | Full (IdP-verified) | During operation |
| Long-running rollouts | Delegation SAs + TokenRequest | SA identity (correlatable) | At creation only |
| Any credential failure | PausedAuth | N/A (paused) | To resume |
| GitOps | Signed intent | Full (cryptographic) | At signing only |

For non-K8s targets, the delivery agent declares what credential type it needs and handles the target-specific mechanics (AssumeRole, token exchange, etc). The platform provides the user's identity information and any stored credential references.
