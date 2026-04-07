package hcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// mockEC2 implements EC2API for testing.
type mockEC2 struct {
	vpcCount    int
	subnetCount int
	eipCount    int
	natCount    int
	rtCount     int
	epCount     int

	createVpcCalls              []ec2.CreateVpcInput
	modifyVpcAttrCalls          []ec2.ModifyVpcAttributeInput
	createDhcpCalls             []ec2.CreateDhcpOptionsInput
	assocDhcpCalls              []ec2.AssociateDhcpOptionsInput
	createIGWCalls              []ec2.CreateInternetGatewayInput
	attachIGWCalls              []ec2.AttachInternetGatewayInput
	createSubnetCalls           []ec2.CreateSubnetInput
	allocateAddrCalls           []ec2.AllocateAddressInput
	createNatCalls              []ec2.CreateNatGatewayInput
	createRTCalls               []ec2.CreateRouteTableInput
	createRouteCalls            []ec2.CreateRouteInput
	assocRTCalls                []ec2.AssociateRouteTableInput
	createVpcEndpointCalls      []ec2.CreateVpcEndpointInput
	createTagsCalls             []ec2.CreateTagsInput
}

func (m *mockEC2) CreateVpc(_ context.Context, in *ec2.CreateVpcInput, _ ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	m.createVpcCalls = append(m.createVpcCalls, *in)
	m.vpcCount++
	return &ec2.CreateVpcOutput{
		Vpc: &ec2types.Vpc{
			VpcId:     aws.String(fmt.Sprintf("vpc-%04d", m.vpcCount)),
			CidrBlock: in.CidrBlock,
		},
	}, nil
}

func (m *mockEC2) ModifyVpcAttribute(_ context.Context, in *ec2.ModifyVpcAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error) {
	m.modifyVpcAttrCalls = append(m.modifyVpcAttrCalls, *in)
	return &ec2.ModifyVpcAttributeOutput{}, nil
}

func (m *mockEC2) CreateDhcpOptions(_ context.Context, in *ec2.CreateDhcpOptionsInput, _ ...func(*ec2.Options)) (*ec2.CreateDhcpOptionsOutput, error) {
	m.createDhcpCalls = append(m.createDhcpCalls, *in)
	return &ec2.CreateDhcpOptionsOutput{
		DhcpOptions: &ec2types.DhcpOptions{
			DhcpOptionsId: aws.String("dopt-0001"),
		},
	}, nil
}

func (m *mockEC2) AssociateDhcpOptions(_ context.Context, in *ec2.AssociateDhcpOptionsInput, _ ...func(*ec2.Options)) (*ec2.AssociateDhcpOptionsOutput, error) {
	m.assocDhcpCalls = append(m.assocDhcpCalls, *in)
	return &ec2.AssociateDhcpOptionsOutput{}, nil
}

func (m *mockEC2) CreateInternetGateway(_ context.Context, in *ec2.CreateInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error) {
	m.createIGWCalls = append(m.createIGWCalls, *in)
	return &ec2.CreateInternetGatewayOutput{
		InternetGateway: &ec2types.InternetGateway{
			InternetGatewayId: aws.String("igw-0001"),
		},
	}, nil
}

func (m *mockEC2) AttachInternetGateway(_ context.Context, in *ec2.AttachInternetGatewayInput, _ ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error) {
	m.attachIGWCalls = append(m.attachIGWCalls, *in)
	return &ec2.AttachInternetGatewayOutput{}, nil
}

func (m *mockEC2) CreateSubnet(_ context.Context, in *ec2.CreateSubnetInput, _ ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	m.createSubnetCalls = append(m.createSubnetCalls, *in)
	m.subnetCount++
	return &ec2.CreateSubnetOutput{
		Subnet: &ec2types.Subnet{
			SubnetId:         aws.String(fmt.Sprintf("subnet-%04d", m.subnetCount)),
			AvailabilityZone: in.AvailabilityZone,
			CidrBlock:        in.CidrBlock,
		},
	}, nil
}

func (m *mockEC2) AllocateAddress(_ context.Context, in *ec2.AllocateAddressInput, _ ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error) {
	m.allocateAddrCalls = append(m.allocateAddrCalls, *in)
	m.eipCount++
	return &ec2.AllocateAddressOutput{
		AllocationId: aws.String(fmt.Sprintf("eipalloc-%04d", m.eipCount)),
	}, nil
}

func (m *mockEC2) CreateNatGateway(_ context.Context, in *ec2.CreateNatGatewayInput, _ ...func(*ec2.Options)) (*ec2.CreateNatGatewayOutput, error) {
	m.createNatCalls = append(m.createNatCalls, *in)
	m.natCount++
	return &ec2.CreateNatGatewayOutput{
		NatGateway: &ec2types.NatGateway{
			NatGatewayId: aws.String(fmt.Sprintf("nat-%04d", m.natCount)),
		},
	}, nil
}

func (m *mockEC2) CreateRouteTable(_ context.Context, in *ec2.CreateRouteTableInput, _ ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error) {
	m.createRTCalls = append(m.createRTCalls, *in)
	m.rtCount++
	return &ec2.CreateRouteTableOutput{
		RouteTable: &ec2types.RouteTable{
			RouteTableId: aws.String(fmt.Sprintf("rtb-%04d", m.rtCount)),
		},
	}, nil
}

func (m *mockEC2) CreateRoute(_ context.Context, in *ec2.CreateRouteInput, _ ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	m.createRouteCalls = append(m.createRouteCalls, *in)
	return &ec2.CreateRouteOutput{Return: aws.Bool(true)}, nil
}

func (m *mockEC2) AssociateRouteTable(_ context.Context, in *ec2.AssociateRouteTableInput, _ ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error) {
	m.assocRTCalls = append(m.assocRTCalls, *in)
	return &ec2.AssociateRouteTableOutput{
		AssociationId: aws.String("rtbassoc-0001"),
	}, nil
}

func (m *mockEC2) CreateVpcEndpoint(_ context.Context, in *ec2.CreateVpcEndpointInput, _ ...func(*ec2.Options)) (*ec2.CreateVpcEndpointOutput, error) {
	m.createVpcEndpointCalls = append(m.createVpcEndpointCalls, *in)
	m.epCount++
	return &ec2.CreateVpcEndpointOutput{
		VpcEndpoint: &ec2types.VpcEndpoint{
			VpcEndpointId: aws.String(fmt.Sprintf("vpce-%04d", m.epCount)),
		},
	}, nil
}

func (m *mockEC2) CreateTags(_ context.Context, in *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	m.createTagsCalls = append(m.createTagsCalls, *in)
	return &ec2.CreateTagsOutput{}, nil
}

// Cleanup stubs — not exercised in create tests but required by the interface.
func (m *mockEC2) DeleteVpc(context.Context, *ec2.DeleteVpcInput, ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
	return &ec2.DeleteVpcOutput{}, nil
}
func (m *mockEC2) DeleteSubnet(context.Context, *ec2.DeleteSubnetInput, ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
	return &ec2.DeleteSubnetOutput{}, nil
}
func (m *mockEC2) DeleteInternetGateway(context.Context, *ec2.DeleteInternetGatewayInput, ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error) {
	return &ec2.DeleteInternetGatewayOutput{}, nil
}
func (m *mockEC2) DetachInternetGateway(context.Context, *ec2.DetachInternetGatewayInput, ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error) {
	return &ec2.DetachInternetGatewayOutput{}, nil
}
func (m *mockEC2) DeleteNatGateway(context.Context, *ec2.DeleteNatGatewayInput, ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error) {
	return &ec2.DeleteNatGatewayOutput{}, nil
}
func (m *mockEC2) ReleaseAddress(context.Context, *ec2.ReleaseAddressInput, ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error) {
	return &ec2.ReleaseAddressOutput{}, nil
}
func (m *mockEC2) DeleteRouteTable(context.Context, *ec2.DeleteRouteTableInput, ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error) {
	return &ec2.DeleteRouteTableOutput{}, nil
}
func (m *mockEC2) DeleteVpcEndpoints(context.Context, *ec2.DeleteVpcEndpointsInput, ...func(*ec2.Options)) (*ec2.DeleteVpcEndpointsOutput, error) {
	return &ec2.DeleteVpcEndpointsOutput{}, nil
}
func (m *mockEC2) DeleteDhcpOptions(context.Context, *ec2.DeleteDhcpOptionsInput, ...func(*ec2.Options)) (*ec2.DeleteDhcpOptionsOutput, error) {
	return &ec2.DeleteDhcpOptionsOutput{}, nil
}
func (m *mockEC2) DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{}, nil
}
func (m *mockEC2) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{}, nil
}
func (m *mockEC2) DescribeNatGateways(context.Context, *ec2.DescribeNatGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return &ec2.DescribeNatGatewaysOutput{}, nil
}

// mockRoute53 implements Route53API for testing.
type mockRoute53 struct {
	zoneCount         int
	createZoneCalls   []route53.CreateHostedZoneInput
}

func (m *mockRoute53) CreateHostedZone(_ context.Context, in *route53.CreateHostedZoneInput, _ ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error) {
	m.createZoneCalls = append(m.createZoneCalls, *in)
	m.zoneCount++
	return &route53.CreateHostedZoneOutput{
		HostedZone: &r53types.HostedZone{
			Id:   aws.String(fmt.Sprintf("/hostedzone/Z%04d", m.zoneCount)),
			Name: in.Name,
		},
	}, nil
}

func (m *mockRoute53) DeleteHostedZone(context.Context, *route53.DeleteHostedZoneInput, ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error) {
	return &route53.DeleteHostedZoneOutput{}, nil
}

func (m *mockRoute53) ListHostedZonesByName(context.Context, *route53.ListHostedZonesByNameInput, ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	return &route53.ListHostedZonesByNameOutput{}, nil
}

func (m *mockRoute53) ListResourceRecordSets(context.Context, *route53.ListResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return &route53.ListResourceRecordSetsOutput{}, nil
}

func (m *mockRoute53) ChangeResourceRecordSets(context.Context, *route53.ChangeResourceRecordSetsInput, ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	return &route53.ChangeResourceRecordSetsOutput{}, nil
}

func TestCreateInfra_SingleZone(t *testing.T) {
	ec2Mock := &mockEC2{}
	r53Mock := &mockRoute53{}

	spec := InfraSpec{
		Name:       "test-cluster",
		InfraID:    "test-infra-123",
		Region:     "us-east-1",
		BaseDomain: "example.com",
		Zones:      []string{"us-east-1a"},
	}

	out, err := CreateInfra(context.Background(), ec2Mock, r53Mock, spec)
	if err != nil {
		t.Fatalf("CreateInfra failed: %v", err)
	}

	// VPC created
	if len(ec2Mock.createVpcCalls) != 1 {
		t.Fatalf("expected 1 CreateVpc call, got %d", len(ec2Mock.createVpcCalls))
	}
	if aws.ToString(ec2Mock.createVpcCalls[0].CidrBlock) != "10.0.0.0/16" {
		t.Errorf("VPC CIDR = %q, want 10.0.0.0/16", aws.ToString(ec2Mock.createVpcCalls[0].CidrBlock))
	}
	if out.VPCID != "vpc-0001" {
		t.Errorf("VPCID = %q, want vpc-0001", out.VPCID)
	}

	// VPC attributes modified (DNS support + hostnames)
	if len(ec2Mock.modifyVpcAttrCalls) != 2 {
		t.Errorf("expected 2 ModifyVpcAttribute calls, got %d", len(ec2Mock.modifyVpcAttrCalls))
	}

	// DHCP options
	if len(ec2Mock.createDhcpCalls) != 1 {
		t.Errorf("expected 1 CreateDhcpOptions call, got %d", len(ec2Mock.createDhcpCalls))
	}
	if out.DHCPOptionsID != "dopt-0001" {
		t.Errorf("DHCPOptionsID = %q, want dopt-0001", out.DHCPOptionsID)
	}

	// IGW
	if len(ec2Mock.createIGWCalls) != 1 {
		t.Errorf("expected 1 CreateInternetGateway call, got %d", len(ec2Mock.createIGWCalls))
	}
	if out.InternetGatewayID != "igw-0001" {
		t.Errorf("InternetGatewayID = %q, want igw-0001", out.InternetGatewayID)
	}

	// 1 private + 1 public subnet
	if len(ec2Mock.createSubnetCalls) != 2 {
		t.Fatalf("expected 2 CreateSubnet calls, got %d", len(ec2Mock.createSubnetCalls))
	}
	if aws.ToString(ec2Mock.createSubnetCalls[0].CidrBlock) != "10.0.0.0/24" {
		t.Errorf("private subnet CIDR = %q, want 10.0.0.0/24", aws.ToString(ec2Mock.createSubnetCalls[0].CidrBlock))
	}
	if aws.ToString(ec2Mock.createSubnetCalls[1].CidrBlock) != "10.0.1.0/24" {
		t.Errorf("public subnet CIDR = %q, want 10.0.1.0/24", aws.ToString(ec2Mock.createSubnetCalls[1].CidrBlock))
	}
	if len(out.PrivateSubnetIDs) != 1 || len(out.PublicSubnetIDs) != 1 {
		t.Errorf("expected 1 private + 1 public subnet, got %d + %d", len(out.PrivateSubnetIDs), len(out.PublicSubnetIDs))
	}

	// NAT gateway
	if len(ec2Mock.createNatCalls) != 1 {
		t.Errorf("expected 1 CreateNatGateway call, got %d", len(ec2Mock.createNatCalls))
	}
	if len(out.NATGatewayIDs) != 1 {
		t.Errorf("expected 1 NAT gateway ID, got %d", len(out.NATGatewayIDs))
	}

	// Route tables: 1 private + 1 public = 2
	if len(ec2Mock.createRTCalls) != 2 {
		t.Errorf("expected 2 CreateRouteTable calls, got %d", len(ec2Mock.createRTCalls))
	}
	if len(out.PrivateRouteTableIDs) != 1 {
		t.Errorf("expected 1 private route table, got %d", len(out.PrivateRouteTableIDs))
	}
	if out.PublicRouteTableID == "" {
		t.Error("expected non-empty public route table ID")
	}

	// Routes: 1 private (0.0.0.0/0 -> NAT) + 1 public (0.0.0.0/0 -> IGW) = 2
	if len(ec2Mock.createRouteCalls) != 2 {
		t.Errorf("expected 2 CreateRoute calls, got %d", len(ec2Mock.createRouteCalls))
	}

	// S3 endpoint
	if len(ec2Mock.createVpcEndpointCalls) != 1 {
		t.Errorf("expected 1 CreateVpcEndpoint call, got %d", len(ec2Mock.createVpcEndpointCalls))
	}
	if out.S3EndpointID != "vpce-0001" {
		t.Errorf("S3EndpointID = %q, want vpce-0001", out.S3EndpointID)
	}
	expectedService := "com.amazonaws.us-east-1.s3"
	if aws.ToString(ec2Mock.createVpcEndpointCalls[0].ServiceName) != expectedService {
		t.Errorf("S3 endpoint service = %q, want %q", aws.ToString(ec2Mock.createVpcEndpointCalls[0].ServiceName), expectedService)
	}

	// DNS zones: 2 (private + local)
	if len(r53Mock.createZoneCalls) != 2 {
		t.Fatalf("expected 2 CreateHostedZone calls, got %d", len(r53Mock.createZoneCalls))
	}
	if out.PrivateDNSZoneID == "" {
		t.Error("expected non-empty PrivateDNSZoneID")
	}
	if out.LocalDNSZoneID == "" {
		t.Error("expected non-empty LocalDNSZoneID")
	}
	if aws.ToString(r53Mock.createZoneCalls[0].Name) != "test-infra-123.example.com" {
		t.Errorf("private zone name = %q, want test-infra-123.example.com", aws.ToString(r53Mock.createZoneCalls[0].Name))
	}
	if aws.ToString(r53Mock.createZoneCalls[1].Name) != "test-infra-123.hypershift.local" {
		t.Errorf("local zone name = %q, want test-infra-123.hypershift.local", aws.ToString(r53Mock.createZoneCalls[1].Name))
	}

	// Zones preserved
	if len(out.Zones) != 1 || out.Zones[0] != "us-east-1a" {
		t.Errorf("Zones = %v, want [us-east-1a]", out.Zones)
	}

	// Verify tagging: VPC should have the cluster ownership tag
	vpcTags := ec2Mock.createVpcCalls[0].TagSpecifications[0].Tags
	foundClusterTag := false
	for _, tag := range vpcTags {
		if aws.ToString(tag.Key) == "kubernetes.io/cluster/test-infra-123" && aws.ToString(tag.Value) == "owned" {
			foundClusterTag = true
			break
		}
	}
	if !foundClusterTag {
		t.Error("VPC missing kubernetes.io/cluster/test-infra-123: owned tag")
	}
}
