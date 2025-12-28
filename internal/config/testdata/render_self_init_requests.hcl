initialize "enable-stdout-audit" {
  request "enable-stdout-audit-request" {
    operation = "update"
    path      = "sys/audit/stdout"
    data {
      options = {
        file_path = "stdout"
      }
      type = "file"
    }
  }
}
