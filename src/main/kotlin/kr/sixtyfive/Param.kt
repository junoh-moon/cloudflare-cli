package kr.sixtyfive

data class Param(
	val type: String,
	val name: String,
	val content: String,
	val ttl: Int,
	val proxied: Boolean
)
