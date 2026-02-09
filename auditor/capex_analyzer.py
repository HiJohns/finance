#!/usr/bin/env python3
"""
Mag 7 CAPEX Analyzer

分析 Mag 7 科技巨头的 AI 投入效率和财务健康状况。
重点关注：
- AI 投入强度 (CAPEX / Revenue)
- 自由现金流收益率 (FCF Yield)
- CAPEX 与 Revenue 增速对比趋势
- 泡沫风险预警
"""

import yfinance as yf
import pandas as pd
import numpy as np
from datetime import datetime
from typing import Dict, List
import warnings
warnings.filterwarnings('ignore')


# Mag 7 股票代码
MAG7_TICKERS = {
    'MSFT': 'Microsoft',
    'GOOGL': 'Alphabet (Google)',
    'AMZN': 'Amazon',
    'META': 'Meta Platforms',
    'AAPL': 'Apple',
    'TSLA': 'Tesla',
    'NVDA': 'NVIDIA'
}


class Mag7Analyzer:
    """Mag 7 财务分析器"""
    
    def __init__(self):
        self.data = {}
        self.results = []
        
    def fetch_financial_data(self, ticker: str) -> Dict:
        """
        获取股票财务数据
        
        Returns:
            Dict 包含：
            - quarterly_income_stmt: 季度损益表
            - quarterly_cash_flow: 季度现金流量表
            - info: 公司基本信息
        """
        try:
            print(f"   正在连接 Yahoo Finance API...")
            stock = yf.Ticker(ticker)
            
            # 获取季度损益表
            income_stmt = stock.quarterly_income_stmt
            print(f"   损益表列数: {len(income_stmt.columns) if income_stmt is not None else 0}")
            
            # 获取季度现金流量表
            cash_flow = stock.quarterly_cash_flow
            print(f"   现金流表列数: {len(cash_flow.columns) if cash_flow is not None else 0}")
            
            # 获取公司信息
            info = stock.info
            
            return {
                'ticker': ticker,
                'name': MAG7_TICKERS.get(ticker, ticker),
                'income_stmt': income_stmt,
                'cash_flow': cash_flow,
                'info': info,
                'market_cap': info.get('marketCap', 0)
            }
        except Exception as e:
            print(f"❌ 获取 {ticker} 数据失败: {e}")
            import traceback
            traceback.print_exc()
            return None
    
    def format_quarter(self, date) -> str:
        """格式化季度字符串"""
        if isinstance(date, datetime):
            quarter = (date.month - 1) // 3 + 1
            return f"{date.year}-Q{quarter}"
        return str(date)
    
    def extract_quarterly_metrics(self, data: Dict) -> pd.DataFrame:
        """
        从财务报表中提取季度指标
        
        提取：
        - Revenue (营业收入)
        - CapitalExpenditure (资本支出)
        - FreeCashFlow (自由现金流)
        """
        ticker = data['ticker']
        income_stmt = data['income_stmt']
        cash_flow = data['cash_flow']
        
        metrics = []
        
        try:
            # 获取季度日期列（最近4个季度）
            if income_stmt is None or income_stmt.empty:
                print(f"⚠️ {ticker}: 无法获取损益表数据")
                return pd.DataFrame()
            
            quarters = list(income_stmt.columns[:4])  # 最近4个季度
            print(f"   可用季度: {[self.format_quarter(q) for q in quarters]}")
            
            for quarter in quarters:
                quarter_str = self.format_quarter(quarter)
                
                try:
                    # 从损益表获取 Revenue
                    revenue = None
                    revenue_fields = ['TotalRevenue', 'Revenue', 'Total Revenue']
                    for field in revenue_fields:
                        if field in income_stmt.index:
                            revenue = income_stmt.loc[field, quarter]
                            break
                    
                    # 从现金流量表获取 Capital Expenditure
                    capex = None
                    capex_fields = ['CapitalExpenditure', 'Capital Expenditures', 'PurchaseOfPPE', 
                                   'Purchase of Property Plant and Equipment', 'Capital Expenditure']
                    for field in capex_fields:
                        if field in cash_flow.index:
                            capex = cash_flow.loc[field, quarter]
                            break
                    
                    # 从现金流量表获取 Free Cash Flow
                    fcf = None
                    fcf_fields = ['FreeCashFlow', 'Free Cash Flow']
                    for field in fcf_fields:
                        if field in cash_flow.index:
                            fcf = cash_flow.loc[field, quarter]
                            break
                    
                    # 如果找不到 FCF，尝试计算
                    if fcf is None:
                        ocf = None
                        ocf_fields = ['OperatingCashFlow', 'Total Cash From Operating Activities', 
                                     'Cash Flow From Operating Activities']
                        for field in ocf_fields:
                            if field in cash_flow.index:
                                ocf = cash_flow.loc[field, quarter]
                                break
                        
                        if ocf is not None and capex is not None:
                            fcf = ocf + capex  # capex 通常是负数
                    
                    print(f"   {quarter_str}: Revenue={revenue is not None}, CAPEX={capex is not None}, FCF={fcf is not None}")
                    
                    metrics.append({
                        'Ticker': ticker,
                        'Company': data['name'],
                        'Quarter': quarter_str,
                        'Quarter_Date': quarter,
                        'Revenue': float(revenue) if revenue is not None and not pd.isna(revenue) else None,
                        'CAPEX': abs(float(capex)) if capex is not None and not pd.isna(capex) else None,
                        'FCF': float(fcf) if fcf is not None and not pd.isna(fcf) else None
                    })
                except Exception as e:
                    print(f"⚠️ {ticker} {quarter_str}: 提取指标失败 - {e}")
                    continue
            
        except Exception as e:
            print(f"❌ {ticker}: 处理财务报表失败 - {e}")
            import traceback
            traceback.print_exc()
        
        return pd.DataFrame(metrics)
    
    def calculate_ratios(self, df: pd.DataFrame) -> pd.DataFrame:
        """计算关键财务比率"""
        if df.empty:
            return df
        
        # CAPEX 强度 = CAPEX / Revenue
        df['CAPEX_Ratio'] = df.apply(
            lambda row: row['CAPEX'] / row['Revenue'] 
            if pd.notna(row['CAPEX']) and pd.notna(row['Revenue']) and row['Revenue'] != 0 
            else None,
            axis=1
        )
        
        return df
    
    def analyze_trends(self, df: pd.DataFrame) -> Dict:
        """
        分析过去四个季度的趋势
        
        Returns:
            Dict 包含趋势分析结果和预警信息
        """
        if df.empty or len(df) < 2:
            return {'status': 'insufficient_data', 'warning': False, 'ticker': df['Ticker'].iloc[0] if not df.empty else 'Unknown'}
        
        # 按时间排序
        df = df.sort_values('Quarter_Date')
        
        # 计算增速（季度环比）
        df['Revenue_Growth'] = df['Revenue'].pct_change() * 100
        df['CAPEX_Growth'] = df['CAPEX'].pct_change() * 100
        
        # 分析最近4个季度的趋势
        recent_data = df.tail(4)
        
        # 计算平均增速
        avg_revenue_growth = recent_data['Revenue_Growth'].mean()
        avg_capex_growth = recent_data['CAPEX_Growth'].mean()
        
        # 判断是否有泡沫风险
        # 条件：CAPEX 增速持续远超 Revenue 增速（连续2个季度以上）
        warning_quarters = 0
        for idx in range(1, len(recent_data)):
            row = recent_data.iloc[idx]
            if pd.notna(row['CAPEX_Growth']) and pd.notna(row['Revenue_Growth']):
                # CAPEX 增长超过 Revenue 增长 20 个百分点以上
                if row['CAPEX_Growth'] - row['Revenue_Growth'] > 20:
                    warning_quarters += 1
        
        has_bubble_risk = warning_quarters >= 2
        
        # 计算最新 CAPEX 强度
        latest_capex_ratio = df['CAPEX_Ratio'].iloc[-1] if not df.empty else None
        
        return {
            'ticker': df['Ticker'].iloc[0],
            'company': df['Company'].iloc[0],
            'quarters_analyzed': len(df),
            'avg_revenue_growth': avg_revenue_growth,
            'avg_capex_growth': avg_capex_growth,
            'latest_capex_ratio': latest_capex_ratio,
            'warning_quarters': warning_quarters,
            'bubble_risk': has_bubble_risk,
            'status': 'success'
        }
    
    def print_company_report(self, ticker: str, metrics_df: pd.DataFrame, analysis: Dict):
        """打印单个公司的分析报告"""
        print("\n" + "="*80)
        print(f"📊 {analysis.get('company', ticker)} ({ticker})")
        print("="*80)
        
        if metrics_df.empty:
            print("❌ 无可用数据")
            return
        
        # 打印季度数据表格
        print("\n📈 最近四个季度财务指标:")
        print("-" * 80)
        print(f"{'Quarter':<15} {'Revenue':>18} {'CAPEX':>18} {'FCF':>18} {'CAPEX%':>10}")
        print("-" * 80)
        
        for _, row in metrics_df.iterrows():
            rev = f"${row['Revenue']/1e9:.2f}B" if pd.notna(row['Revenue']) else "N/A"
            capex = f"${row['CAPEX']/1e9:.2f}B" if pd.notna(row['CAPEX']) else "N/A"
            fcf = f"${row['FCF']/1e9:.2f}B" if pd.notna(row['FCF']) else "N/A"
            ratio = f"{row['CAPEX_Ratio']*100:.1f}%" if pd.notna(row['CAPEX_Ratio']) else "N/A"
            
            print(f"{row['Quarter']:<15} {rev:>18} {capex:>18} {fcf:>18} {ratio:>10}")
        
        print("-" * 80)
        
        # 打印趋势分析
        print(f"\n📊 趋势分析 (过去 {analysis.get('quarters_analyzed', 0)} 个季度):")
        if pd.notna(analysis.get('avg_revenue_growth')):
            print(f"   • 平均营收增速: {analysis['avg_revenue_growth']:+.1f}%")
        else:
            print(f"   • 平均营收增速: N/A")
            
        if pd.notna(analysis.get('avg_capex_growth')):
            print(f"   • 平均 CAPEX 增速: {analysis['avg_capex_growth']:+.1f}%")
        else:
            print(f"   • 平均 CAPEX 增速: N/A")
            
        if pd.notna(analysis.get('latest_capex_ratio')):
            print(f"   • 最新 AI 投入强度: {analysis['latest_capex_ratio']*100:.1f}%")
        else:
            print(f"   • 最新 AI 投入强度: N/A")
        
        # 泡沫风险预警
        if analysis.get('bubble_risk'):
            print("\n" + "🚨"*40)
            print(f"🚨 泡沫风险预警！")
            print(f"🚨 CAPEX 增速连续 {analysis.get('warning_quarters', 0)} 个季度远超 Revenue 增速")
            print(f"🚨 AI 投入效率下降，存在资本配置风险")
            print("🚨"*40)
        elif analysis.get('warning_quarters', 0) > 0:
            print(f"\n⚠️  轻度预警: {analysis['warning_quarters']} 个季度 CAPEX 增速超过 Revenue")
        else:
            print("\n✅ 财务状况健康，CAPEX 与 Revenue 增长匹配")
    
    def generate_summary_report(self, all_analysis: List[Dict]):
        """生成汇总报告"""
        print("\n\n" + "="*80)
        print("📋 MAG 7 AI 投入效率综合评估报告")
        print("="*80)
        print(f"📅 分析时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        print(f"📊 数据范围: 最近四个季度")
        print("="*80)
        
        # 过滤有效分析结果
        valid_analysis = [a for a in all_analysis if a.get('status') == 'success']
        
        if not valid_analysis:
            print("❌ 未能获取有效的财务数据")
            return
        
        # 按 AI 投入强度排序
        sorted_analysis = sorted(
            valid_analysis,
            key=lambda x: x.get('latest_capex_ratio', 0) if pd.notna(x.get('latest_capex_ratio')) else 0,
            reverse=True
        )
        
        print("\n🏆 AI 投入强度排行榜 (CAPEX / Revenue):")
        print("-" * 60)
        for i, analysis in enumerate(sorted_analysis, 1):
            ratio = analysis.get('latest_capex_ratio')
            ratio_str = f"{ratio*100:.1f}%" if pd.notna(ratio) else "N/A"
            risk_indicator = "🔴" if analysis.get('bubble_risk') else "🟢"
            print(f"{i}. {analysis.get('company', 'Unknown'):<25} {ratio_str:>8} {risk_indicator}")
        
        # 风险公司列表
        risky_companies = [a for a in valid_analysis if a.get('bubble_risk')]
        if risky_companies:
            print("\n" + "🚨"*40)
            print("⚠️  高风险公司列表 (CAPEX 增速持续超过 Revenue):")
            for company in risky_companies:
                print(f"   • {company.get('company', 'Unknown')} ({company.get('ticker', 'Unknown')})")
                rev_growth = company.get('avg_revenue_growth')
                capex_growth = company.get('avg_capex_growth')
                rev_str = f"{rev_growth:+.1f}%" if pd.notna(rev_growth) else "N/A"
                capex_str = f"{capex_growth:+.1f}%" if pd.notna(capex_growth) else "N/A"
                print(f"     CAPEX 增速: {capex_str} | Revenue 增速: {rev_str}")
            print("🚨"*40)
        else:
            print("\n✅ 所有 Mag 7 公司目前未发现明显的 CAPEX 泡沫风险")
        
        print("\n" + "="*80)
        print("💡 投资建议:")
        print("   • 关注 CAPEX 强度过高且增速远超营收增速的公司")
        print("   • 优先考虑 FCF Yield 高且资本配置效率好的标的")
        print("   • 密切监控 AI 投资的实际转化率和 ROI")
        print("   • 2026 年初数据重点关注 Q4 2025 财报")
        print("="*80)
    
    def run_analysis(self):
        """运行完整分析"""
        print("\n" + "🚀"*40)
        print("🚀 启动 Mag 7 AI 投入效率分析系统")
        print("🚀"*40)
        print(f"\n📊 分析标的: {', '.join(MAG7_TICKERS.keys())}")
        print("📊 数据来源: Yahoo Finance (季度财务报表)")
        print("📊 分析重点: 2025-2026 财年最新季度数据\n")
        
        all_analysis = []
        
        for ticker, name in MAG7_TICKERS.items():
            print(f"\n{'='*80}")
            print(f"⏳ 正在获取 {name} ({ticker}) 的财务数据...")
            print('='*80)
            
            # 获取数据
            data = self.fetch_financial_data(ticker)
            if data is None:
                print(f"❌ {ticker}: 数据获取失败")
                continue
            
            # 提取指标
            metrics_df = self.extract_quarterly_metrics(data)
            if metrics_df.empty:
                print(f"⚠️ {ticker}: 无法提取有效指标")
                continue
            
            print(f"   成功提取 {len(metrics_df)} 个季度数据")
            
            # 计算比率
            metrics_df = self.calculate_ratios(metrics_df)
            
            # 趋势分析
            analysis = self.analyze_trends(metrics_df)
            all_analysis.append(analysis)
            
            # 打印公司报告
            self.print_company_report(ticker, metrics_df, analysis)
        
        # 生成汇总报告
        self.generate_summary_report(all_analysis)


def main():
    """主函数"""
    analyzer = Mag7Analyzer()
    analyzer.run_analysis()


if __name__ == "__main__":
    main()
