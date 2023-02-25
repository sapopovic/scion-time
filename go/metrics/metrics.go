package metrics

const (
	ServerReqsServedInterleavedH = "The total number of requests served in interleaved mode"
	ServerReqsServedInterleavedN = "timeservice_server_reqs_served_interleaved"
	ServerRxtIncrementsH         = "The total number of RX timestamps incremented to ensure monotonicity"
	ServerRxtIncrementsN         = "timeservice_server_rxt_increments"
	ServerTssItemsH              = "The total number of timestamp store items stored (one item per client)"
	ServerTssItemsN              = "timeservice_server_tss_items"
	ServerTssValuesH             = "The total number of timestamp store values stored"
	ServerTssValuesN             = "timeservice_server_tss_values"
	ServerTxtIncrementsAfterH    = "The total number of TX timestamps incremented after transfer to ensure monotonicity"
	ServerTxtIncrementsAfterN    = "timeservice_server_txt_increments_after"
	ServerTxtIncrementsBeforeH   = "The total number of TX timestamps incremented before transfer to ensure monotonicity"
	ServerTxtIncrementsBeforeN   = "timeservice_server_txt_increments_before"

	IPServerPktsReceivedH = "The total number of packets received via IP"
	IPServerPktsReceivedN = "timeservice_ip_server_pkts_received"
	IPServerReqsAcceptedH = "The total number of requests accepted via IP"
	IPServerReqsAcceptedN = "timeservice_ip_server_reqs_accepted"
	IPServerReqsServedH   = "The total number of requests served via IP"
	IPServerReqsServedN   = "timeservice_ip_server_reqs_served"

	SCIONServerPktsAuthenticatedH = "The total number of packets authenticated via SCION"
	SCIONServerPktsAuthenticatedN = "timeservice_scion_server_pkts_authenticated"
	SCIONServerPktsForwardedH     = "The total number of packets forwarded via SCION"
	SCIONServerPktsForwardedN     = "timeservice_scion_server_pkts_forwarded"
	SCIONServerPktsReceivedH      = "The total number of packets received via SCION"
	SCIONServerPktsReceivedN      = "timeservice_scion_server_pkts_received"
	SCIONServerReqsAcceptedH      = "The total number of requests accepted via SCION"
	SCIONServerReqsAcceptedN      = "timeservice_scion_server_reqs_accepted"
	SCIONServerReqsServedH        = "The total number of requests served via SCION"
	SCIONServerReqsServedN        = "timeservice_scion_server_reqs_served"

	SyncGlobalCorrH = "The current clock correction applied based on global sync"
	SyncGlobalCorrN = "timeservice_sync_global_corr"
	SyncLocalCorrH  = "The current clock correction applied based on local sync"
	SyncLocalCorrN  = "timeservice_sync_local_corr"
)
