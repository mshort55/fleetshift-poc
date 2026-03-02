package domain

// TargetID uniquely identifies a target within the platform.
type TargetID string

// TargetType identifies the kind of target and determines which
// [DeliveryAgent] handles delivery. Built-in types include "kubernetes",
// "platform", and "local"; addons register additional types.
type TargetType string

// DeploymentID uniquely identifies a deployment.
type DeploymentID string
