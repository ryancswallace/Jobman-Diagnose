"use strict";

function priceInvoice(invoice) {
  return invoice.total.toFixed(2) + " " + invoice.currency.code;
}

process.stderr.write("pricing invoice INV-778\n");
priceInvoice({ total: 42, currency: undefined });
