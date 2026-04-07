import React from 'react'

/**
 * Displays active phone request streams.
 * Each caller shows: phone icon + digits entered so far (or result).
 *
 * Expected `callers` prop format:
 * [
 *   { id: "call-1", digits: "10", status: "dialling" },
 *   { id: "call-2", digits: "986", status: "success" },
 *   { id: "call-3", digits: "999", status: "fail" },
 * ]
 *
 * status: "dialling" | "success" | "fail"
 */
export default function RequestDigits({ callers }) {
  if (!callers || callers.length === 0) return null

  return (
    <div className="request-digits">
      {callers.map((caller) => (
        <div key={caller.id} className="request-caller">
          <div className="request-phone-icon">&#9742;</div>
          {caller.status === 'success' ? (
            <span className="request-result success">Thanx!</span>
          ) : caller.status === 'fail' ? (
            <span className="request-result fail">Try again</span>
          ) : (
            <span className="request-digits-text">{caller.digits}</span>
          )}
        </div>
      ))}
    </div>
  )
}
