package kr.sixtyfive

import com.github.ajalt.clikt.core.CliktCommand
import com.github.ajalt.clikt.core.CliktError
import com.github.ajalt.clikt.parameters.options.flag
import com.github.ajalt.clikt.parameters.options.option
import com.github.ajalt.clikt.parameters.options.required

class Cli : CliktCommand(name = "cloudflare") {
	private val email by option("--email", metavar = "<email address>").required()
	private val key by option("--key", metavar = "<secret key>").required()
	private val zoneName by option("--zone", metavar = "<zone name>")
	private val dnsName by option("--dns", metavar = "<dns record name>")

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
				val dnsName = dnsName ?: throw CliktError("--dns option is required")
				val zoneName = zoneName ?: throw CliktError("--zone option is required")
				Cloudflare.getIp()
					.let { UpdateParams("A", dnsName, it, 120, false) }
					.let { cloudflare.updateRecord(zoneName, it) }
			}
		}
	}
}

fun main(args: Array<String>) = Cli().main(args)