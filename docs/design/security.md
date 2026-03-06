# Security model

Principles:

- No to little built-in trust of platform components (no god mode service accounts or keys)
- End to end user identity everywhere – auditable, no confused deputy

## Generic targets vs kubernetes specifics

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

- git ops – GitOps has a platform level indirection: the git repo is the authority, and the platform applies from there. Some tools may support tenant-specific service accounts or impersonation.
- Audience scoping – if we want to scope tokens to particular clusters, we need separate audiences for those. More IdP configuration to do. Hard to make dynamic.
- Reconciliation – this is similar to the gitops challenge. 

## Bootstrapping targets

When targets (e.g. clusters) are bootstrapped we may necessarily have elevated privileges at that point for that target (e.g. a kubeconfig or a privileged user). Under that identity we assume we can bootstrap other configs like RBAC syncing. These could perhaps be their own deployments or part of the cluster deployment itself.

## Long running rollouts (e.g. approval gates) / deployment invalidation

Deployments are long-lived intents in a sense. The rollout may take arbitrarily long. An invalidation signals can come arbitrarily late. This retriggers the deployment.

We can confidently detect these signals but require re-approval to continue. That's maybe not so bad actually but does mean lots of approval baby sitting. What you want is essentially some durable tightly scoped approval that tracks the user's own permissions over time.

So how to store durable tightly scoped approval?

### Service accounts specifically for delegation

When something is long running, the user creates a service account dedicated to run on their behalf, with a scoped subset of their permissions.

The platform then asks a cluster for a token for that service account (rather than storing keys) and gets a short-lived token it uses to do work when the user isn't present.

Ideally: 

- Something expires these over time
- When the user's permissions restricts to less than their shadow service accounts, it automatically restricts the permissions of those service accounts

You could also "just" create specific service accounts to run workloads that you wanted long-running, with strict permissions. If they ever tried to escape that, the deployment pauses for approval.

### Refresh tokens

These are credentials and tough to store.

Ideally you'd:

- Sender constrain them. This makes the platform privileged but only its protected private key. Leaked credentials are not a problem. Sender constrained refresh tokens have some support. It would require the backend to be a confidential client and not the frontend. That can complicate CLI integration. Maybe you only approve these long lived flows through the browser, though. It's a few-time operation.
- Scope them. This can be hard because it requires more IdP configuration e.g. client per cluster which could be awful without automation. And automating that is itself difficult to set up (dynamic client registration / aud configuration). Plus you'd want token exchange of some kind or the original aud needs to include every cluster.

### Constrained impersonation

This is conceptually similar to the above, but means the platform directly impersonates the user. The downside is the platform still has to be trusted to make claims about the user due to impersonated groups.

## IdP orchestration

In various scenarios, we could benefit from specific IdP configuration:

- Per cluster client IDs (audiences)
- Permission-level scoping (assuming you have an authorizer which takes this into account)
- If an IdP can handle the refresh token route... setup for that

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

One thing that could help matters is if there was a CI check that ran authorization through on the platform level – this could probably catch a lot.

The bigger challenge is securely storing longer lived credentials. See "Refresh tokens" above.

### Signed intent

A more promising model might be to have something cluster-side that operates on "signed intents."

1. A manifest in git is accompanied by a signature and a revision (ideally w/ provenance via hash)
2. The server is only authorized to put "intent" resources into a cluster
3. A controller or admission validates the signature, extracts the original user, authorizes against that, and unwraps the manifest and applies that if succesful.

That _controller_ or _admission_ is now very privileged, but only to that cluster. Admission is privileged by design.