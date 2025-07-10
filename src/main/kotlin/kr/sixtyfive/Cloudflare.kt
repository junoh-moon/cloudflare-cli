package kr.sixtyfive

import com.google.gson.GsonBuilder
import io.github.oshai.kotlinlogging.KotlinLogging
import org.http4k.client.ApacheClient
import org.http4k.core.Method
import org.http4k.core.Request


class Cloudflare(email: String, key: String) {
	private val logger = KotlinLogging.logger { }

	private val client = ApacheClient()
	private val json = GsonBuilder()
		.setPrettyPrinting()
		.disableHtmlEscaping()
		.create()

	private val endpoint = "https://api.cloudflare.com/client/v4"
	private val header = mapOf(
		"X-Auth-Email" to email,
		"X-Auth-Key" to key,
		"Content-Type" to "application/json",
	)

	companion object {
		fun getIp(): String {
			return ApacheClient()(Request(Method.GET, "http://ipinfo.io/ip")).bodyString()
		}
	}

	fun listZones(): Boolean {
		val url = "$endpoint/zones"

		return get(url)
			.let { json.fromJson(it, Map::class.java) }
			.let { it["result"] as? List<*> }
			?.fold(Table().addLine()) { acc, entry ->
				val m = entry as? Map<*, *>
				acc.addRow(m?.get("name") ?: "null")
					.addLine()
			}?.render()
			?.let(::println)
			?.let { true } ?: false
	}

	fun listDnsRecordsByName(zoneName: String, pretty: Boolean = false): Boolean {
		return getZoneId(zoneName)
			?.let { listDnsRecords(it, pretty) } ?: false
	}

	private fun getZoneId(zoneName: String): String? {
		val url = "$endpoint/zones"
		return get(url)
			.let { searchId(it, zoneName) }
	}

	fun listDnsRecords(zoneId: String, pretty: Boolean = false): Boolean {
		val url = "$endpoint/zones/$zoneId/dns_records"
		return get(url)
			.let { json.fromJson(it, Map::class.java) }
			.let { it["result"] as? List<*> }
			?.map { it as Map<*, *> }
			?.fold(if (pretty) Table().addLine() else Table()) { acc, map ->
				acc.addRow(map["name"] ?: "")
					.let { if (pretty) it.addLine() else it }
			}?.let { if (pretty) it.addLine() else it }
			?.render()
			?.let(::println)
			?.let { true } ?: false

	}

	fun updateRecord(zoneName: String, params: Param): Boolean {
		val zoneId = getZoneId(zoneName)
		val dnsId = zoneId?.let { "${endpoint}/zones/${it}/dns_records" }
			?.let(this::get)
			?.let { getDnsId(it, params.name) }

		val content = json.toJson(params)
		return zoneId?.let {
			if (dnsId.isNullOrEmpty()) {
				"$endpoint/zones/$zoneId/dns_records"
					.let { Request(Method.POST, it) }
			} else {
				"$endpoint/zones/$zoneId/dns_records/$dnsId"
					.let { Request(Method.PUT, it) }
			}.headers(header.toList())
				.body(content)
		}
			?.let(client)
			?.bodyString()
			?.apply { logger.info { this } }
			?.let { json.fromJson(it, Map::class.java) }
			?.let { json.toJson(it) }
			?.let(::println)
			?.let { true } ?: let {
			logger.warn { "Update failed" }
			false
		}
	}

	private fun getDnsId(resp: String, dnsName: String): String? {
		return searchId(resp, dnsName)
	}

	private fun get(url: String): String {
		return Request(Method.GET, url)
			.headers(header.toList())
			.let(client)
			.bodyString()
	}

	private fun searchId(resp: String, key: String): String? {
		return resp
			.let { json.fromJson(it, Map::class.java) }
			.let { it["result"] as? List<*> }
			?.map { it as Map<*, *> }
			?.firstOrNull { it["name"] == key }
			?.let { it["id"] as String }
	}
}