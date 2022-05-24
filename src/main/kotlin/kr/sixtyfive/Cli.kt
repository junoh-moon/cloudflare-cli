package kr.sixtyfive

import com.github.ajalt.clikt.core.CliktCommand
import com.github.ajalt.clikt.core.CliktError
import com.github.ajalt.clikt.parameters.options.flag
import com.github.ajalt.clikt.parameters.options.option
import com.github.ajalt.clikt.parameters.options.required

class Cli : CliktCommand(name = "cloudflare") {
	private val email by option("--email", metavar = "EMAIL").required()
	private val key by option("--key", metavar = "TOKEN").required()
	private val zoneName by option("--zone", metavar = "ZONE")
	private val dns by option("--dns", metavar = "DNS(,DNS)*")

	private val listZones by option("--list_zones").flag(default = false)
	private val listDnsRecords by option("--list_dns").flag(default = false)
	private val update by option("--update").flag(default = false)

	override fun run() {
		val cloudflare = Cloudflare(email, key)
		when {
			listZones -> cloudflare.listZones()
			listDnsRecords -> {
				val zoneName = zoneName ?: throw CliktError("--zone option is required")
				cloudflare.listDnsRecordsByName(zoneName)
			}
			update -> {
				val dnsList = dns?.split(",") ?: throw CliktError("--dns option is required")
				val zoneName = zoneName ?: throw CliktError("--zone option is required")
				val ip = Cloudflare.getIp()
				dnsList.map {
					val dnsName = if (it.isNotBlank()) "$it.$zoneName" else zoneName
					val record = Param("A", dnsName, ip, 120, false)
					cloudflare.updateRecord(zoneName, record)
				}
			}
		}
	}
}

fun main(args: Array<String>) = Cli().main(args)