import os
import smtplib
import yfinance as yf
import pandas as pd
import numpy as np
from email.mime.text import MIMEText
from email.header import Header

# --- 核心配置（建议在服务器环境变量中设置） ---
SMTP_HOST = "smtp.163.com"
SMTP_PORT = 465  # SSL 端口
SMTP_USER = os.getenv("SMTP_USER")      # 你的163邮箱地址
SMTP_PASS = os.getenv("SMTP_PASS")      # 163邮箱的“授权码”（非登录密码）
RECEIVER_EMAIL = os.getenv("RECEIVER_EMAIL") # 接收报告的邮箱

def send_email_report(subject, content):
    """通过 163 SMTP 发送分析报告"""
    try:
        message = MIMEText(content, 'plain', 'utf-8')
        message['From'] = SMTP_USER
        message['To'] = RECEIVER_EMAIL
        message['Subject'] = Header(subject, 'utf-8')

        with smtplib.SMTP_SSL(SMTP_HOST, SMTP_PORT) as server:
            server.login(SMTP_USER, SMTP_PASS)
            server.sendmail(SMTP_USER, [RECEIVER_EMAIL], message.as_string())
        print("✅ 审计报告已发送至邮箱。")
    except Exception as e:
        print(f"❌ 邮件发送失败: {e}")

def analyze_risk_and_correlation():
    # 你的核心美元资产
    assets = ["AMD", "SLV", "USO", "GLD", "IWY", "SRVR"]
    dxy_ticker = "DX-Y.NYB"
    
    # 获取数据
    data = yf.download(assets + [dxy_ticker], period="6mo", interval="1d")['Close']
    returns = data.pct_change().dropna()

    report_lines = ["--- Beacon 系统资产审计报告 ---", f"日期: {pd.Timestamp.now()}\n"]
    alert_triggered = False

    # 1. 计算各资产与美元的相关性
    report_lines.append("【美元相关性审计】")
    for asset in assets:
        corr = returns[asset].corr(returns[dxy_ticker])
        status = "⚠️ 强相关" if corr < -0.6 else "🟢 独立运动"
        report_lines.append(f"{asset} vs DXY: {corr:.4f} ({status})")
        
        # 如果白银或 AMD 这种高 Beta 资产突然被美元锁定，触发预警
        if asset in ["AMD", "SLV"] and corr < -0.65:
            alert_triggered = True

    # 2. 计算风险指标 (Volatility & MDD)
    report_lines.append("\n【资产风险体检】")
    for asset in assets:
        vol = returns[asset].std() * np.sqrt(252)
        # 计算 MDD
        cum_rets = (1 + returns[asset]).cumprod()
        mdd = ((cum_rets - cum_rets.expanding().max()) / cum_rets.expanding().max()).min()
        report_lines.append(f"{asset}: Vol={vol:.2%}, MDD={mdd:.2%}")

    content = "\n".join(report_lines)
    
    # 3. 决定是否发送报告（可以是定时发送，也可以是触发告警时发送）
    subject = "【Beacon 预警】发现资产与美元相关性异常" if alert_triggered else "【Beacon 定期】资产风险审计周报"
    send_email_report(subject, content)

if __name__ == "__main__":
    analyze_risk_and_correlation()
