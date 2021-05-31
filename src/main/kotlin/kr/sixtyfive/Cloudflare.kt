package kr.sixtyfive

import com.google.gson.GsonBuilder
import org.http4k.client.ApacheClient
import org.http4k.core.Method
import org.http4k.core.Request

fun getIp(): String {
	return ApacheClient()(Request(Method.GET, "http://ipinfo.io/ip")).bodyString()
}


class Cloudflare(email: String, key: String) {
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
		val url = "$endpoint/zones"
		return get(url)
			.let { searchId(it, zoneName) }
			?.let { listDnsRecords(it, pretty) } ?: false
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
			?.first { it["name"] == key }
			?.let { it["id"] as String }
	}
}