#!/usr/bin/env python3
from typing import Union
from requests import get, put, Response
from json import loads, dumps
from dataclasses import dataclass, asdict
from argparse import ArgumentParser

from requests.api import post


@dataclass
class DataType:
	type: str
	name: str
	content: str
	ttl: int
	proxied: bool


def get_ip():
	return get("http://ipinfo.io/ip").text


class Cloudflare:
	endpoint = "https://api.cloudflare.com/client/v4"

	def __init__(self, email: str, key: str) -> None:
		self.header = {
		    "X-Auth-Email": email,
		    "X-Auth-Key": key,
		    "Content-Type": "application/json",
		}
		return

	def list_zones(self):
		url = f"{self.endpoint}/zones"
		resp = self.get(url)
		print([it["name"] for it in loads(resp.text)["result"]])
		return

	def list_dns_records(self, *, zone_name: str = None, zone_id: str = None):
		if not zone_name and not zone_id:
			raise RuntimeError("At least one of arguments must be provided")
		elif zone_name:
			resp = self.get(f"{self.endpoint}/zones")
			zone_id = self.search_id(resp, zone_name)

		url = f"{self.endpoint}/zones/{zone_id}/dns_records"

		resp = self.get(url)
		print([it["name"] for it in loads(resp.text)["result"]])
		return

	def get(self, url: str, verbose=False):
		resp = get(url, headers=self.header)
		if verbose:
			self.prettyprint(resp)
		return resp

	def put(self, url: str, data: DataType, verbose=False):
		resp = put(url, headers=self.header, json=asdict(data))
		if verbose:
			self.prettyprint(resp)
		return resp

	def post(self, url: str, data: DataType, verbose=False):
		resp = post(url, headers=self.header, json=asdict(data))
		if verbose:
			self.prettyprint(resp)
		return resp

	@staticmethod
	def prettyprint(resp: Response):
		print(dumps(loads(resp.text), indent=2))

	@staticmethod
	def search_id(resp: Response, name_key: str) -> Union[str, None]:
		return next(
		    (it["id"]
		     for it in loads(resp.text)["result"] if it["name"] == name_key),
		    None)


def main(args):
	cf = Cloudflare(args.email, args.key)
	if args.update:
		if not args.zone_id:
			url = f"{Cloudflare.endpoint}/zones"
			resp = cf.get(url)
			args.zone_id = Cloudflare.search_id(resp, args.zone_name)

		if not args.dns_id:
			url = f"{Cloudflare.endpoint}/zones/{args.zone_id}/dns_records"
			resp = cf.get(url)
			args.dns_id = Cloudflare.search_id(resp, args.dns_name)
		if not args.ip:
			args.ip = get_ip()

		if args.dns_id is None:
			# Regard as DNS creation
			url = f"{Cloudflare.endpoint}/zones/{args.zone_id}/dns_records"
			cf.post(url,
			        DataType("A", args.dns_name, args.ip, 120, False),
			        verbose=True)
		else:
			url = f"{Cloudflare.endpoint}/zones/{args.zone_id}/dns_records/{args.dns_id}"
			cf.put(url,
			       DataType("A", args.dns_name, args.ip, 120, False),
			       verbose=True)
	elif args.list_zones:
		cf.list_zones()
	elif args.list_dns_records:
		if args.zone_id:
			cf.list_dns_records(zone_id=args.zone_id)
		elif args.zone_name:
			cf.list_dns_records(zone_name=args.zone_name)
		else:
			raise RuntimeError(
			    "At least one of zone_id or zone_name is required to list dns records"
			)
	return


if __name__ == "__main__":
	parser = ArgumentParser()
	parser.add_argument("--email", required=True)
	parser.add_argument("--key", required=True)

	parser.add_argument("--update",
	                    help="Update dns records",
	                    action="store_true")
	parser.add_argument("--list_zones", help="List zones", action="store_true")
	parser.add_argument("--list_dns_records",
	                    help="List dns records",
	                    action="store_true")

	parser.add_argument("--zone_name")
	parser.add_argument("--zone_id")

	parser.add_argument("--dns_name")
	parser.add_argument("--dns_id")

	parser.add_argument("--ip", help="Use public IP if not explicitly set.")

	main(parser.parse_args())
