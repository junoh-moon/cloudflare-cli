package kr.sixtyfive

data class UpdateParams(
	val type: String,
	val name: String,
	val content: String,
	val ttl: Int,
	val proxied: Boolean
)
