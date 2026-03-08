import sys
import json
import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
import numpy as np
from datetime import datetime

def annotate_news_events(ax, data, names, x_positions):
    """Annotate high impact news events on the chart"""
    if 'news_events' not in data:
        return
    
    news_events = data.get('news_events', [])
    high_impact_events = [e for e in news_events if e.get('impact_score', 0) >= 0.8]
    
    if not high_impact_events:
        return
    
    # Add event markers on the chart
    for i, name in enumerate(names):
        for event in high_impact_events:
            # Check if event is related to this asset
            related_symbols = event.get('related_symbols', [])
            if name in related_symbols or not related_symbols:
                # Plot a red dot marker
                ax.plot(x_positions[i], 0.85, 'ro', markersize=12, 
                       markeredgecolor='darkred', markeredgewidth=2, 
                       label='High Impact News' if i == 0 else "")
                
                # Add tooltip-like annotation
                title = event.get('title', 'Unknown')[:30] + '...'
                ax.annotate(f"📰 {title}", 
                           xy=(x_positions[i], 0.85),
                           xytext=(10, 10), textcoords='offset points',
                           fontsize=7, color='red',
                           bbox=dict(boxstyle='round,pad=0.3', facecolor='yellow', alpha=0.7))

def get_risk_light_color(risk_level):
    """Get color for geopolitical risk indicator"""
    colors = {
        'Green': '#28a745',
        'Yellow': '#ffc107', 
        'Red': '#dc3545'
    }
    return colors.get(risk_level, '#6c757d')

def main():
    try:
        data = json.loads(sys.stdin.read())
        
        if 'corrs6m' not in data:
            print("Missing corrs6m data", file=sys.stderr)
            return
            
        names = list(data['corrs6m'].keys())
        values6m = [v[0] for v in data['corrs6m'].values()]
        
        values30 = []
        for name in names:
            if name in data.get('corrs30', {}):
                values30.append(data['corrs30'][name][0])
            else:
                values30.append(0)
        
        vix_dxy_corr = data.get('vix_dxy_corr', 0)
        risk_level = data.get('geopolitical_risk', 'Green')
        
        x = np.arange(len(names))
        width = 0.35
        
        fig, ax = plt.subplots(figsize=(14, 8))
        
        ax.bar(x - width/2, values6m, width, label='6mo Baseline', color='#87CEEB')
        
        colors30 = []
        for i, v30 in enumerate(values30):
            v6m = values6m[i]
            if v30 < -0.6 or v30 < (v6m - 0.2):
                colors30.append('#FF4500')
            else:
                colors30.append('#4682B4')
        
        ax.bar(x + width/2, values30, width, label='30d Current', color=colors30)
        
        # Annotate high impact news events
        annotate_news_events(ax, data, names, x)
        
        ax.axhline(y=-0.7, color='red', linestyle='--', linewidth=1.5)
        
        ax.set_ylabel('Correlation')
        ax.set_title('IronCore 2.0: Asset Correlation Trend Audit with Geopolitical Overlay')
        ax.set_xticks(x)
        ax.set_xticklabels(names, rotation=45, ha='right')
        ax.legend(loc='upper right')
        ax.set_ylim(-1, 1)
        ax.grid(axis='y', linestyle='--', alpha=0.7)
        
        for i, v in enumerate(values6m):
            ax.annotate(f'{v:.3f}', 
                       xy=(x[i] - width/2, v),
                       xytext=(0, -10 if v < 0 else 3),
                       textcoords='offset points',
                       ha='center', va='top' if v < 0 else 'bottom', fontsize=8)
        
        for i, v in enumerate(values30):
            if v != 0:
                ax.annotate(f'{v:.3f}', 
                           xy=(x[i] + width/2, v),
                           xytext=(0, -10 if v < 0 else 3),
                           textcoords='offset points',
                           ha='center', va='top' if v < 0 else 'bottom', fontsize=8)
        
        # VIX-DXY correlation box
        vix_color = 'red' if vix_dxy_corr > 0.5 else 'green'
        props = dict(boxstyle='round', facecolor='wheat', alpha=0.8)
        ax.text(0.02, 0.98, f'VIX-DXY Corr: {vix_dxy_corr:.3f}', 
                transform=ax.transAxes, fontsize=10, verticalalignment='top',
                bbox=props, color=vix_color, fontweight='bold')
        
        if vix_dxy_corr > 0.5:
            ax.text(0.02, 0.93, '⚠️ Liquidity Black Hole', 
                    transform=ax.transAxes, fontsize=9, verticalalignment='top',
                    color='red', fontweight='bold')
        
        # Geopolitical Risk Light
        risk_color = get_risk_light_color(risk_level)
        risk_text = f"🌍 Geopolitical Risk: {risk_level}"
        ax.text(0.98, 0.98, risk_text, 
                transform=ax.transAxes, fontsize=11, verticalalignment='top',
                horizontalalignment='right',
                bbox=dict(boxstyle='round,pad=0.5', facecolor=risk_color, alpha=0.9, edgecolor='black', linewidth=2),
                color='white' if risk_level == 'Red' else 'black',
                fontweight='bold')
        
        # Add legend for news events
        if data.get('news_events'):
            red_dot = mpatches.Patch(color='red', label='⚠️ High Impact News (Score≥0.8)')
            ax.legend(handles=[red_dot], loc='lower left', fontsize=8)
        
        plt.tight_layout()
        plt.savefig("audit_chart.png", dpi=150, bbox_inches='tight')
        print("Chart saved: audit_chart.png", file=sys.stderr)
    except Exception as e:
        print(f"Plotting error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc(file=sys.stderr)

if __name__ == "__main__":
    main()