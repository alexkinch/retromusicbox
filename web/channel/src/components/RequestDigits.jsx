import React from 'react'

/**
 * Displays active phone request streams.
 * Each caller shows: phone icon + digits entered so far (or result).
 *
 * status: "dialling" | "success" | "fail"
 */
export default function RequestDigits({ callers }) {
  if (!callers || callers.length === 0) return null

  return (
    <div className="request-digits">
      {callers.map((caller) => (
        <div key={caller.id} className="ticker-line ticker-line-dial">
          <span className="request-phone-icon">&#9742;</span>
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
