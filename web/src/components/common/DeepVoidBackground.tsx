import React from 'react'

interface DeepVoidBackgroundProps extends React.HTMLAttributes<HTMLDivElement> {
    children?: React.ReactNode
    className?: string
    disableAnimation?: boolean
}

export function DeepVoidBackground({ children, className = '', disableAnimation = false, ...props }: DeepVoidBackgroundProps) {
    return (
        <div className={`relative w-full min-h-screen bg-nofx-bg text-nofx-text overflow-hidden flex flex-col ${className}`} {...props}>
            {/* Background layers: use a much lighter static stack when animations are disabled */}
            {disableAnimation ? (
                <>
                    <div className="absolute inset-0 pointer-events-none z-0 bg-[radial-gradient(circle_at_top,rgba(212,255,51,0.055),transparent_42%),linear-gradient(180deg,#131313,#0b0b0b)]"></div>
                    <div className="absolute inset-0 pointer-events-none z-0 opacity-[0.04] bg-[linear-gradient(to_right,rgba(255,255,255,0.05)_1px,transparent_1px),linear-gradient(to_bottom,rgba(255,255,255,0.05)_1px,transparent_1px)] bg-[size:36px_36px]"></div>
                </>
            ) : (
                <>
                    {/* 1. Grain/Noise Texture */}
                    <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20 mix-blend-soft-light pointer-events-none fixed z-0"></div>

                    {/* 2. 单层透视网格（去掉重复平铺网格，画面更干净） */}
                    <div className="absolute inset-0 pointer-events-none fixed z-0">
                        <div className="absolute inset-x-0 bottom-0 h-[50vh] bg-[linear-gradient(to_right,rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(to_bottom,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:40px_40px] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)] opacity-[0.35]" style={{ transform: 'perspective(500px) rotateX(60deg) translateY(100px) scale(2)' }}></div>
                    </div>

                    {/* 3. Ambient Glow Spots */}
                    <div className="absolute inset-0 overflow-hidden pointer-events-none fixed z-0">
                        <div className="absolute top-[-10%] left-[-10%] w-[40vw] h-[40vw] bg-nofx-gold/10 rounded-full blur-[120px] mix-blend-screen animate-pulse-slow"></div>
                        <div className="absolute bottom-[-10%] right-[-10%] w-[40vw] h-[40vw] bg-nofx-accent/5 rounded-full blur-[120px] mix-blend-screen animate-pulse-slow" style={{ animationDelay: '2s' }}></div>
                    </div>

                    {/* 4. 极轻扫描线（降低强度，避免和内容「打架」） */}
                    <div className="absolute inset-0 pointer-events-none fixed z-[5] opacity-[0.14]">
                        <div className="absolute inset-0 bg-[linear-gradient(rgba(18,16,16,0)_50%,rgba(0,0,0,0.12)_50%),linear-gradient(90deg,rgba(255,0,0,0.03),rgba(0,255,0,0.01),rgba(0,0,255,0.03))] bg-[length:100%_4px,3px_100%] pointer-events-none"></div>
                    </div>
                </>
            )}

            {/* Content Layer */}
            <div className="relative z-10 flex min-h-0 flex-1 flex-col w-full">
                {children}
            </div>
        </div>
    )
}
