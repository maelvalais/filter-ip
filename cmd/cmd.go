//
// main.go
// Copyright (C) 2019 Maël Valais <mael.valais@gmail.com>
//
// Distributed under terms of the MIT license.
//

package cmd

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/couchbase/gocb"
	"github.com/spf13/cobra"
)

// Vlan is an "IP network" in the LBP context, i.e., it corresponds to one
// specific VLAN.
type Vlan struct {
	VlanID     string    `json:"vlanId"`
	SubnetIpv4 uint32    `json:"subnetIpv4"`
	MaskIpv4   uint32    `json:"maskIpv4"`
	IPNet      net.IPNet `json:"-"`
}

// GetVlans extracts the valsn from Couchbase
func GetVlans(givenIp string) ([]*Vlan, error) {
	const BUCKET = "netops-dev"
	client, err := gocb.Connect("http://localhost:8091/")

	if err != nil {
		log.Fatal(err)
	}

	bucket, err := client.OpenBucket(BUCKET, BUCKET)
	if err != nil {
		log.Fatal("when connecting to couchbase: ", err)
	}
	if bucket == nil {
		log.Fatal("couchbase connection went wrong")
	}
	bucket.Manager("", "").CreatePrimaryIndex("", true, false)

	// Remove all data from the bucket
	{
		_, err := bucket.ExecuteN1qlQuery(gocb.NewN1qlQuery("delete from `netops-dev`"), []interface{}{})
		if err != nil {
			log.Fatal(err)
		}
	}

	ipnetsStr := []string{
		"192.168.1.1/28",
		"192.168.1.28/28",
		"192.168.1.80/28",
		"192.168.1.250/28",
	}

	// Turn ip net strings into a slice of IPNet
	ipnets := make(map[string]*net.IPNet)
	for _, ipStr := range ipnetsStr {
		_, ipnet, err := net.ParseCIDR(ipStr)
		if err != nil {
			log.Printf("skipping '%v' as it doesn't seem to be a valid CIDR (RFC 4632, RFC 4291)", ipStr)
			continue
		}
		ipnets[ipStr] = ipnet
	}

	for str, ipnet := range ipnets {
		bucket.Upsert(str,
			&Vlan{str, ip2int(ipnet.IP), ip2int(net.IP(ipnet.Mask)), *ipnet}, 0,
		)
	}

	{
		rows, err := bucket.ExecuteN1qlQuery(gocb.NewN1qlQuery(
			"select maskIpv4, subnetIpv4, vlanId "+
				"from `netops-dev`"), []interface{}{})
		if err != nil {
			log.Fatal(err)
		}
		var vlan Vlan
		for rows.Next(&vlan) {
			vlan.IPNet = net.IPNet{
				IP:   int2ip(vlan.SubnetIpv4),
				Mask: net.IPMask(int2ip(vlan.MaskIpv4)),
			}
			fmt.Printf("%v\n", vlan.IPNet)
		}
	}

	ipStr := givenIp
	ip := ip2int(net.ParseIP(ipStr))
	rows, err := bucket.ExecuteN1qlQuery(gocb.NewN1qlQuery(
		"select maskIpv4, subnetIpv4, vlanId "+
			"from `netops-dev` "+
			"where bitand($1, maskIpv4) = subnetIpv4"), []interface{}{ip})
	if err != nil {
		log.Fatal(err)
	}
	var vlan Vlan
	var vlans []Vlan
	for rows.Next(&vlan) {
		vlan.IPNet = net.IPNet{
			IP:   int2ip(vlan.SubnetIpv4),
			Mask: net.IPMask(int2ip(vlan.MaskIpv4)),
		}
		vlans = append(vlans, vlan)
	}
	fmt.Printf("lookup for '%v': ", ipStr)
	for _, vlan := range vlans {
		fmt.Printf("%v ", vlan.IPNet.String())
	}
	fmt.Print("\n")
	return nil, nil
}

//Execute is the main function run by Cobra, the CLI library.
func Execute() {
	var cli = &cobra.Command{
		Use:  "filter-ip",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			GetVlans(args[0])
			// var file = os.Stdin
			// if len(args) > 0 {
			// 	var err error
			// 	file, err = os.Open(args[0])
			// 	if err != nil {
			// 		log.Fatal(err)
			// 	}
			// 	defer file.Close()
			// }

			// scanner := bufio.NewScanner(file)

			// var ips []string
			// for scanner.Scan() {
			// 	ips = append(ips, scanner.Text())
			// }

			// if err := scanner.Err(); err != nil {
			// 	log.Println(err)
			// }
		},
	}

	if err := cli.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// https://gist.github.com/ammario/649d4c0da650162efd404af23e25b86b
func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func int2ip(nn uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, nn)
	return ip
}
